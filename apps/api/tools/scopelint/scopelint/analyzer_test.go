package scopelint_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"teka/apps/api/tools/scopelint/scopelint"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	const root = "teka/apps/api/internal/features/testcase"
	analysistest.Run(t, testdata, scopelint.Analyzer,
		root, root+"/centers", root+"/testutil", root+"/middleware", root+"/authctx", root+"/seeds")
}
