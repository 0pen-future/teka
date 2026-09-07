// Command scopelint runs the scopelint analyzer standalone: go run
// ./tools/scopelint ./internal/...
package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"teka/apps/api/tools/scopelint/scopelint"
)

func main() {
	singlechecker.Main(scopelint.Analyzer)
}
