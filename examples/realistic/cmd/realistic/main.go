// Package main prints the realistic-workload module graph as a
// smoke test. Builds the application via app.Build and lists
// every wired module by name. PLAN §7 Phase 6.1.
package main

import (
	"fmt"
	"log"

	"github.com/blockberries/punnet-sdk/examples/realistic/app"
)

func main() {
	r, err := app.Build()
	if err != nil {
		log.Fatalf("build app: %v", err)
	}
	fmt.Printf("ChainID: %s\n", r.App.ChainID())
	fmt.Printf("Modules: %d\n", len(r.App.Router().Modules()))
	for _, mod := range r.App.Router().Modules() {
		fmt.Printf("  - %s\n", mod.Name())
	}
}
