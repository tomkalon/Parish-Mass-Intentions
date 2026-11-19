package web

import (
	"bytes"
	"html/template"
	"log"
	"net/http"
)

var previewTemplate = template.Must(template.ParseFS(templateFS, "templates/preview.html"))

type previewView struct {
	Title     string
	BackURL   string
	BackLabel string
	Note      string
	PDFURL    string
	Action    string
	Hidden    []previewField
	Checks    []previewCheck
	Save      *previewSave
	Message   string
}

type previewField struct {
	Name  string
	Value string
}

type previewCheck struct {
	Name     string
	Label    string
	Checked  bool
	Disabled bool
}

type previewSave struct {
	Action string
	Hidden []previewField
}

func renderPreview(w http.ResponseWriter, view previewView) {
	var buf bytes.Buffer
	err := previewTemplate.Execute(&buf, view)
	if err != nil {
		log.Printf("error rendering pdf preview: %v", err)
		serverError(w)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err = buf.WriteTo(w)
	if err != nil {
		log.Printf("error writing response: %v", err)
	}
}

func writePDFResponse(w http.ResponseWriter, buf *bytes.Buffer, filename string) {
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="`+filename+`"`)

	_, err := buf.WriteTo(w)
	if err != nil {
		log.Printf("error writing response: %v", err)
	}
}
