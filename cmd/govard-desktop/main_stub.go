//go:build !desktop

package main

import "fmt"

func main() {
	fmt.Println("Govard Desktop is not built yet. Run `govard desktop --dev`, or build it with `go build -tags desktop,production -o bin/govard-desktop ./cmd/govard-desktop`.")
}
