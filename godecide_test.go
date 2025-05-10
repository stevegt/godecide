package tree

import (
	"embed"
	"fmt"

	// "io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	yaml "gopkg.in/yaml.v2"
)

// Use embed to load example YAML files.
// The examples directory is one level up from the tree directory.
//
//go:embed examples/*.yaml
var testFS embed.FS

// testWarn returns a Warn function for testing that logs warnings via t.Log.
func testWarn(t *testing.T) Warn {
	return func(args ...interface{}) {
		// If multiple arguments are passed, assume the first is a format string.
		if len(args) > 1 {
			t.Logf(fmt.Sprint(args...))
		} else if len(args) == 1 {
			t.Log(fmt.Sprint(args[0]))
		}
	}
}

func TestFromYAML(t *testing.T) {
	// Read one of the example files: college.yaml
	data, err := testFS.ReadFile("examples/college.yaml")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	roots, err := FromYAML(data)
	if err != nil {
		t.Fatalf("FromYAML failed: %v", err)
	}
	if len(roots) == 0 {
		t.Fatalf("Expected at least one root node, got none")
	}
	// Check that at least one root has a non-empty name.
	found := false
	for _, a := range roots {
		if a.Name != "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("No ast with valid name found")
	}
}

func TestRecalcAndForwardBackward(t *testing.T) {
	// Use the hbr example to test recalculation.
	data, err := testFS.ReadFile("examples/hbr.yaml")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	roots, err := FromYAML(data)
	if err != nil {
		t.Fatalf("FromYAML failed: %v", err)
	}
	// Define a fixed time for the simulation.
	now := time.Date(2023, time.January, 1, 9, 0, 0, 0, time.UTC)
	// Recalculate the timelines.
	Recalc(roots, now, testWarn(t))
	// Check that for each AST, the Forward pass has set Start and End properly.
	for _, a := range roots {
		if a.Start.Before(now) {
			t.Errorf("Node %s: start time %v is before now %v", a.Name, a.Start, now)
		}
		if a.End.Before(a.Start) {
			t.Errorf("Node %s: end time %v is before start time %v", a.Name, a.End, a.Start)
		}
		// Also check that expected values for leaf nodes have been set.
		if len(a.Hyperedges) == 0 {
			if a.Expected.Cash != a.Path.Cash {
				t.Errorf("Node %s: expected cash %v does not match path cash %v", a.Name, a.Expected.Cash, a.Path.Cash)
			}
		}
	}
}

func TestToDot(t *testing.T) {
	// Use the college example to test DOT generation.
	data, err := testFS.ReadFile("examples/college.yaml")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	roots, err := FromYAML(data)
	if err != nil {
		t.Fatalf("FromYAML failed: %v", err)
	}
	// Set a fixed time.
	now := time.Date(2023, time.January, 1, 9, 0, 0, 0, time.UTC)
	Recalc(roots, now, testWarn(t))
	// Generate DOT output in left-to-right mode (tb false).
	dotBytes := ToDot(roots, testWarn(t), false)
	if len(dotBytes) == 0 {
		t.Error("ToDot produced empty output")
	}
	dotStr := string(dotBytes)
	// Check for some Graphviz keywords in the DOT output.
	if !strings.Contains(dotStr, "digraph") && !strings.Contains(dotStr, "graph") {
		t.Error("DOT output does not contain expected graph keywords")
	}
}

func TestCatExample(t *testing.T) {
	// Test CatExample using the college example.
	buf, err := CatExample(testFS, "example:college")
	if err != nil {
		t.Fatalf("CatExample failed: %v", err)
	}
	if len(buf) == 0 {
		t.Error("CatExample returned empty content")
	}
	// Check that the returned content contains an expected key.
	if !strings.Contains(string(buf), "college:") {
		t.Error("CatExample output does not contain expected content")
	}
}

func TestLsExamples(t *testing.T) {
	out := LsExamples(testFS)
	if len(out) == 0 {
		t.Error("LsExamples returned empty string")
	}
	// Check that the output lists known example names, e.g. college.
	if !strings.Contains(out, "example:college") {
		t.Error("LsExamples output does not list 'example:college'")
	}
}

func TestToYAML(t *testing.T) {
	// Test the Nodes.ToYAML function using the college example.
	data, err := testFS.ReadFile("examples/college.yaml")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	var nodes Nodes
	err = yaml.Unmarshal(data, &nodes)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	yamlOut, err := nodes.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML failed: %v", err)
	}
	// Unmarshal the produced YAML back into a map and compare keys.
	var nodes2 Nodes
	err = yaml.Unmarshal(yamlOut, &nodes2)
	if err != nil {
		t.Fatalf("Unmarshal of YAML output failed: %v", err)
	}
	if len(nodes) != len(nodes2) {
		t.Errorf("Expected same number of nodes, got %d and %d", len(nodes), len(nodes2))
	}
}

func TestRootNodes(t *testing.T) {
	// Test the RootNodes method using the college example.
	data, err := testFS.ReadFile("examples/college.yaml")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	var nodes Nodes
	err = yaml.Unmarshal(data, &nodes)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	roots := nodes.RootNodes()
	if len(roots) == 0 {
		t.Error("No root nodes found")
	}
	// Verify that none of the returned root nodes appear as children.
	for _, node := range nodes {
		for child := range node.Paths {
			if _, ok := roots[child]; ok {
				t.Errorf("Child node %s found in root nodes", child)
			}
		}
	}
}

func TestGenerateOutputFiles(t *testing.T) {
	// This test compares the generated DOT output from example YAML files with expected output files.
	// If the environment variable GEN_TEST_OUTPUT is set (to any non-empty value), then the expected
	// output files are generated/updated in the testdata directory.
	genOutput := os.Getenv("GEN_TEST_OUTPUT")
	exampleFiles := []string{"college.yaml", "duedates.yaml", "hbr.yaml"}

	// Ensure testdata directory exists.
	err := os.MkdirAll("testdata", 0755)
	if err != nil {
		t.Fatalf("Failed to create testdata directory: %v", err)
	}

	for _, fname := range exampleFiles {
		data, err := testFS.ReadFile("examples/" + fname)
		if err != nil {
			t.Errorf("ReadFile for %s failed: %v", fname, err)
			continue
		}
		roots, err := FromYAML(data)
		if err != nil {
			t.Errorf("FromYAML for %s failed: %v", fname, err)
			continue
		}
		// Use a fixed time stamp for reproducibility.
		now := time.Date(2023, time.January, 1, 9, 0, 0, 0, time.UTC)
		Recalc(roots, now, testWarn(t))
		dotBytes := ToDot(roots, testWarn(t), false)

		expectedFile := filepath.Join("testdata", strings.TrimSuffix(fname, ".yaml")+".dot")
		if genOutput != "" {
			err = os.WriteFile(expectedFile, dotBytes, 0644)
			if err != nil {
				t.Errorf("Failed to write expected output file %s: %v", expectedFile, err)
			} else {
				t.Logf("Generated output file: %s", expectedFile)
			}
		} else {
			expected, err := os.ReadFile(expectedFile)
			if err != nil {
				// If expected file does not exist, indicate to generate it.
				t.Errorf("Failed to read expected output %s: %v", expectedFile, err)
				continue
			}
			// Normalize whitespace and remove variable coordinate attributes for comparison.
			genStr := normalizeDot(string(dotBytes))
			expStr := normalizeDot(string(expected))
			if genStr != expStr {
				t.Errorf("DOT output for %s does not match expected output", fname)
			}
		}
	}
}

// normalizeDot removes Graphviz coordinate attributes that may vary between runs.
func normalizeDot(input string) string {
	rePos := regexp.MustCompile(`pos="[^"]*"`)
	res := rePos.ReplaceAllString(input, "")
	reBB := regexp.MustCompile(`bb="[^"]*"`)
	res = reBB.ReplaceAllString(res, "")
	reRects := regexp.MustCompile(`rects="[^"]*"`)
	res = reRects.ReplaceAllString(res, "")
	return strings.TrimSpace(res)
}

// YAMLUnmarshal is a helper that wraps yaml.Unmarshal.
// Since the godecide.go file uses gopkg.in/yaml.v2 and a dot-import for goadapt,
// we provide a local helper here.
func YAMLUnmarshal(in []byte, out interface{}) error {
	// Use the same YAML library that godecide.go uses.
	// Here we import gopkg.in/yaml.v2 as yaml.
	return yamlUnmarshal(in, out)
}

// yamlUnmarshal is a simple wrapper for yaml.Unmarshal.
func yamlUnmarshal(in []byte, out interface{}) error {
	return yaml.Unmarshal(in, out)
}
