package golinters

import (
	"fmt"
	"sync"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"github.com/incu6us/goimports-reviser/v3/pkg/module"
	"github.com/incu6us/goimports-reviser/v3/reviser"
	"golang.org/x/tools/go/analysis"

	"github.com/golangci/golangci-lint/pkg/config"
	"github.com/golangci/golangci-lint/pkg/golinters/goanalysis"
	"github.com/golangci/golangci-lint/pkg/lint/linter"
)

const goimportsReviserName = "goimports-reviser"

func NewGoimportsReviser(settings *config.GoimportsReviserSettings) *goanalysis.Linter {
	var mu sync.Mutex
	var resIssues []goanalysis.Issue

	analyzer := &analysis.Analyzer{
		Name: goimportsReviserName,
		Doc:  goanalysis.TheOnlyanalyzerDoc,
		Run:  goanalysis.DummyRun,
	}

	return goanalysis.NewLinter(
		goimportsReviserName,
		"goimports-reviser sorts your imports by configured order and makes a basic code formatting",
		[]*analysis.Analyzer{analyzer},
		nil,
	).WithContextSetter(func(lintCtx *linter.Context) {
		analyzer.Run = func(pass *analysis.Pass) (any, error) {
			mu.Lock()
			defer mu.Unlock()

			issues, err := run(lintCtx, settings, pass)
			if err != nil {
				return nil, err
			}

			if len(issues) == 0 {
				return nil, nil
			}

			resIssues = append(resIssues, issues...)

			return nil, nil
		}
	}).WithIssuesReporter(func(*linter.Context) []goanalysis.Issue {
		return resIssues
	}).WithLoadMode(goanalysis.LoadModeSyntax)
}

func run(linterCtx *linter.Context, settings *config.GoimportsReviserSettings, pass *analysis.Pass) ([]goanalysis.Issue, error) {
	fileNames := getFileNames(pass)

	var issues []goanalysis.Issue

	for _, f := range fileNames {
		projectName, err := module.DetermineProjectName("", f)
		if err != nil {
			return nil, err
		}

		formattedContent, originalContent, hasChanged, err := reviser.NewSourceFile(projectName, f).Fix()
		if err != nil {
			return nil, err
		}

		if !hasChanged {
			continue
		}

		fileURI := span.URIFromPath(f)
		edits := myers.ComputeEdits(fileURI, string(originalContent), string(formattedContent))
		unifiedEdits := gotextdiff.ToUnified(f, f, string(originalContent), edits)

		extractedIssues, err := extractIssuesFromPatch(fmt.Sprint(unifiedEdits), linterCtx, goimportsReviserName)
		if err != nil {
			return nil, fmt.Errorf("can't extract issues from gofmt diff output %q: %w", fmt.Sprint(unifiedEdits), err)
		}

		for _, issue := range extractedIssues {
			issues = append(issues, goanalysis.NewIssue(&issue, pass))
		}
	}

	return issues, nil
}
