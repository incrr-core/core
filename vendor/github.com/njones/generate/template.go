package generate

import "text/template"

var buildTmpl = template.Must(template.New("").Parse(`
	// Code generated by go generate; DO NOT EDIT.
	// This file was generated at {{ .Timestamp }}

	package main

	var (
		verDate   = "{{ .BldDate }}"
		verHash   = "{{ .BldHash }}"
		verBuild  = "{{ .BldNumber }}"
		verSemVer = "{{ .BldSemVer }}"
	)
`))

var buildInternTmpl = template.Must(template.New("").Parse(`
	// Code generated by go generate; DO NOT EDIT.
	// This file was generated at {{ .Timestamp }}

	// +build ignore
	
	package main

	// The internal building information

	var extHash = "{{ .ExtHash }}"
	var extHashMap = map[string]struct{ Sig, Dig, J, T string }{ // Signature and Digest...
		{{- range $idx, $fn := .ExtHashSlice }}
		"{{ $fn }}" : {
			T:   "{{ (index $.ExtHashMap $fn).Kind }}",
			Sig: "{{ (index $.ExtHashMap $fn).Sig }}",
			Dig: "{{ (index $.ExtHashMap $fn).Dig }}",
			J:   {{ printf "%q" (index $.ExtHashMap $fn).JSON }},
		},
		{{- end }}
	}
`))