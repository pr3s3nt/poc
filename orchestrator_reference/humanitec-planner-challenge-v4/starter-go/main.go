package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"challenge.local/humanitec-planner/planner"
)

func main() {
	casePath := flag.String("case", "", "path to case.yaml")
	flag.Parse()
	if *casePath == "" {
		fmt.Fprintln(os.Stderr, "--case is required")
		os.Exit(2)
	}

	result := planner.Run(*casePath)
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
