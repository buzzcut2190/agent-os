package context

import (
	"os"
	"path/filepath"
)

// languageRule maps a marker file/dDir to language detection results.
type languageRule struct {
	Marker       string   // file or dir name to look for
	Language     string
	BuildSystem  string
	EntryFiles   []string
	TestPatterns []string
}

var rules = []languageRule{
	{".go", "Go", "Go Modules", nil, []string{"_test.go"}},
	{"go.mod", "Go", "Go Modules", []string{"main.go"}, []string{"_test.go"}},
	{"go.sum", "Go", "Go Modules", nil, nil},
	{"package.json", "TypeScript", "npm/yarn", []string{"index.ts", "src/index.ts"}, nil},
	{"tsconfig.json", "TypeScript", "tsc", []string{"index.ts", "src/index.ts"}, nil},
	{"requirements.txt", "Python", "pip", []string{"main.py", "app.py", "manage.py"}, []string{"test_", "_test.py"}},
	{"setup.py", "Python", "setuptools", []string{"main.py", "app.py"}, nil},
	{"pyproject.toml", "Python", " Poetry/setuptools", []string{"main.py", "app.py"}, nil},
	{"Cargo.toml", "Rust", "Cargo", []string{"src/main.rs", "src/lib.rs"}, []string{"tests/"}},
	{"pom.xml", "Java", "Maven", []string{"src/main/java"}, nil},
	{"build.gradle", "Java", "Gradle", []string{"src/main/java"}, nil},
	{"Gemfile", "Ruby", "Bundler", []string{"main.rb"}, nil},
	{"*.rb", "Ruby", "", nil, nil},
	{"CMakeLists.txt", "C/C++", "CMake", nil, nil},
	{"Makefile", "C/C++", "Make", nil, nil},
	{"*.py", "Python", "", nil, nil},
	{"*.js", "JavaScript", "", nil, nil},
	{"*.ts", "TypeScript", "", nil, nil},
	{"*.rs", "Rust", "", nil, nil},
	{"*.java", "Java", "", nil, nil},
	{"*.c", "C", "", nil, nil},
	{"*.cpp", "C++", "", nil, nil},
	{"*.rb", "Ruby", "", nil, nil},
}

// DetectLanguages scans rootDir for marker files and returns detected languages.
func DetectLanguages(rootDir string) []ProjectType {
	seen := make(map[string]bool)
	var results []ProjectType

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return nil
	}

	// Build set of top-level files/dirs
	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name()] = true
	}

	// Check marker files first (higher priority)
	for _, rule := range rules {
		if rule.Language == "" || seen[rule.Language] {
			continue
		}
		if names[rule.Marker] {
			seen[rule.Language] = true
			results = append(results, ProjectType{
				Language:    rule.Language,
				BuildSystem: rule.BuildSystem,
				EntryFiles:  findEntries(rootDir, rule.EntryFiles),
			})
		}
	}

	// Fallback: scan for file extensions
	if len(results) == 0 {
		extLang := make(map[string]int)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := filepath.Ext(e.Name())
			for _, rule := range rules {
				if rule.Marker == "*"+ext && rule.Language != "" {
					extLang[rule.Language]++
				}
			}
		}
		for lang := range extLang {
			results = append(results, ProjectType{Language: lang})
		}
	}

	return results
}

func findEntries(rootDir string, candidates []string) []string {
	var found []string
	for _, name := range candidates {
		if _, err := os.Stat(filepath.Join(rootDir, name)); err == nil {
			found = append(found, name)
		}
	}
	return found
}
