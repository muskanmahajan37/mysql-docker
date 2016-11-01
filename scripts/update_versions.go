/*
Command line tool for updating Dockerfiles based on changes to versions.yaml.
*/
package main

import (
	"bytes"
	"flag"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
	"text/template"
)

type Package struct {
	Name    string
	Version string
	Major   string
	Gpg     string
}

type Version struct {
	Dir      string
	Repo     string
	Tags     []string
	Packages []Package
}

type Spec struct {
	From           string
	SharedPackages []Package `yaml:"sharedPackages"`
	Versions       []Version
}

func check(e error) {
	if e != nil {
		log.Fatalf("error: %v", e)
	}
}

var dockerfileTemplateString = `##<autogenerated>##
FROM {{ .From }}
{{ range .Packages }}
ENV {{ .Name | ToUpper }}_VERSION {{ .Version -}}
{{ if .Major }}
ENV {{ .Name | ToUpper }}_MAJOR {{ .Major }}
{{- end -}}
{{- if .Gpg }}
ENV {{ .Name | ToUpper }}_GPG_KEY {{ .Gpg }}
{{ end -}}
{{ end }}
##</autogenerated>##`

type DockerfileTemplateData struct {
	From     string
	Packages []Package
}

func filterPackagesByName(packages []Package, name string) []Package {
	filtered := make([]Package, 0)
	for _, p := range packages {
		if p.Name == name {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func generateText(spec Spec, version Version, tmpl *template.Template) []string {
	data := DockerfileTemplateData{spec.From, version.Packages}

	// Replace placeholder packages with shared packages.
	for i, p := range data.Packages {
		shared := filterPackagesByName(spec.SharedPackages, p.Name)
		if len(shared) > 0 {
			if len(shared) > 1 {
				log.Fatalf(
					"found multiple sharedPackages with the name: %v",
					p.Name)
			}
			data.Packages[i] = shared[0]
		}
	}

	var result bytes.Buffer
	tmpl.Execute(&result, data)
	return strings.Split(result.String(), "\n")
}

func readDockerfile(version Version) []string {
	path := filepath.Join(version.Dir, "Dockerfile")
	content, err := ioutil.ReadFile(path)
	check(err)
	return strings.Split(string(content), "\n")
}

func writeDockerfile(version Version, lines []string) {
	path := filepath.Join(version.Dir, "Dockerfile")
	d := []byte(strings.Join(lines, "\n"))
	err := ioutil.WriteFile(path, d, 0644)
	check(err)
}

func replaceLines(lines []string, replacement []string) []string {
	var first, last int = -1, -1
	for i, line := range lines {
		if strings.Contains(line, "<autogenerated>") {
			first = i
		}
		if strings.Contains(line, "</autogenerated>") {
			last = i
		}
	}
	if first == -1 {
		log.Fatalf("Failed to find <autogenererated> token")
	}
	if last == -1 {
		log.Fatalf("Failed to find </autogenererated> token")
	}
	return append(append(lines[:first], replacement...), lines[last+1:]...)
}

func main() {
	pathPtr := flag.String("f", "versions.yaml", "path to versions.yaml")
	flag.Parse()

	data, err := ioutil.ReadFile(*pathPtr)
	check(err)

	spec := Spec{}
	err = yaml.Unmarshal([]byte(data), &spec)
	check(err)
	fmt.Printf("Parsed versions.yaml:\n%+v\n", spec)

	tmpl, _ := template.
		New("dockerfileTemplate").
		Funcs(template.FuncMap{"ToUpper": strings.ToUpper}).
		Parse(dockerfileTemplateString)

	for _, version := range spec.Versions {
		lines := readDockerfile(version)
		replacement := generateText(spec, version, tmpl)
		replaced := replaceLines(lines, replacement)
		writeDockerfile(version, replaced)
	}
}
