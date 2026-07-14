package report

import (
	"fmt"
	"strings"
)

const (
	pdfPageWidth  = 612
	pdfPageHeight = 792
	pdfMargin     = 36
	pdfFontSize   = 9
	pdfLineHeight = 12
)

// ToPDF renders rows as a minimal single/multi-page Helvetica PDF.
func ToPDF(rows []Row, title string) []byte {
	usableWidth := pdfPageWidth - pdfMargin*2
	maxChars := usableWidth / (pdfFontSize / 2) // fontSize*0.5 per char

	var lines []string
	lines = append(lines, title)
	lines = append(lines, strings.Repeat("=", min(len(title), maxChars)))
	lines = append(lines, "")
	if len(rows) == 0 {
		lines = append(lines, "No packets in this date range.")
	} else {
		for _, r := range rows {
			date := r.Date
			if len(date) > 10 {
				date = date[:10]
			}
			commit := r.Commit
			if len(commit) > 8 {
				commit = commit[:8]
			}
			lines = append(lines, fmt.Sprintf("%s  %s  commit %s  signed:%s", date, r.PacketID, commit, r.Signed))
			ticket := r.Ticket
			if ticket == "" {
				ticket = "(none)"
			}
			lines = append(lines, "  ticket: "+ticket)
			for _, w := range wrapText("goal: "+r.Goal, maxChars-2) {
				lines = append(lines, "  "+w)
			}
			decision := r.Decision
			if decision == "" {
				decision = "(none)"
			}
			for _, w := range wrapText("decision: "+decision, maxChars-2) {
				lines = append(lines, "  "+w)
			}
			lines = append(lines, "")
		}
	}

	linesPerPage := (pdfPageHeight - pdfMargin*2 - pdfFontSize) / pdfLineHeight
	var pages [][]string
	for i := 0; i < len(lines); i += linesPerPage {
		end := i + linesPerPage
		if end > len(lines) {
			end = len(lines)
		}
		pages = append(pages, lines[i:end])
	}
	if len(pages) == 0 {
		pages = append(pages, []string{""})
	}
	return assemblePDF(pages)
}

func wrapText(text string, maxChars int) []string {
	if len(text) <= maxChars {
		return []string{text}
	}
	var out []string
	remaining := text
	for len(remaining) > maxChars {
		breakAt := strings.LastIndex(remaining[:maxChars+1], " ")
		if breakAt <= 0 {
			breakAt = maxChars
		}
		out = append(out, remaining[:breakAt])
		remaining = strings.TrimLeft(remaining[breakAt:], " ")
	}
	if len(remaining) > 0 {
		out = append(out, remaining)
	}
	return out
}

func buildPageContent(lines []string) string {
	y := pdfPageHeight - pdfMargin - pdfFontSize
	parts := []string{"BT", fmt.Sprintf("/F1 %d Tf", pdfFontSize), fmt.Sprintf("%d %d Td", pdfMargin, y)}
	first := true
	for _, line := range lines {
		escaped := escapePDFText(line)
		if first {
			parts = append(parts, fmt.Sprintf("(%s) Tj", escaped))
			first = false
		} else {
			parts = append(parts, fmt.Sprintf("0 -%d Td (%s) Tj", pdfLineHeight, escaped))
		}
	}
	parts = append(parts, "ET")
	return strings.Join(parts, "\n")
}

func escapePDFText(text string) string {
	var b strings.Builder
	for _, r := range text {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '(':
			b.WriteString(`\(`)
		case r == ')':
			b.WriteString(`\)`)
		case r >= 0x20 && r <= 0x7E:
			b.WriteRune(r)
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}

// assemblePDF lays out objects: 1 font, 2 pages, then (content, page) pairs
// per page, catalog last. content i → id 3+2*i, page i → id 4+2*i.
func assemblePDF(pages [][]string) []byte {
	fontObj := "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"

	var contentObjs, pageObjs []string
	for _, pageLines := range pages {
		stream := buildPageContent(pageLines)
		contentObjs = append(contentObjs, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	}
	pageIDs := make([]int, len(pages))
	for i := range pages {
		pageIDs[i] = 4 + 2*i
		contentID := 3 + 2*i
		pageObjs = append(pageObjs, fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %d %d] /Contents %d 0 R /Resources << /Font << /F1 1 0 R >> >> >>",
			pdfPageWidth, pdfPageHeight, contentID))
	}

	kids := make([]string, len(pageIDs))
	for i, id := range pageIDs {
		kids[i] = fmt.Sprintf("%d 0 R", id)
	}
	pagesObj := fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pages))
	catalogID := 3 + len(pages)*2
	catalogObj := "<< /Type /Catalog /Pages 2 0 R >>"

	byID := map[int]string{1: fontObj, 2: pagesObj}
	for i := range pages {
		byID[3+2*i] = contentObjs[i]
		byID[4+2*i] = pageObjs[i]
	}
	byID[catalogID] = catalogObj
	maxID := catalogID

	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offset := b.Len()
	offsets := make([]int, maxID+1)
	for id := 1; id <= maxID; id++ {
		body, ok := byID[id]
		if !ok {
			continue
		}
		obj := fmt.Sprintf("%d 0 obj\n%s\nendobj\n", id, body)
		offsets[id] = offset
		b.WriteString(obj)
		offset += len(obj)
	}

	xrefStart := offset
	var xref strings.Builder
	fmt.Fprintf(&xref, "xref\n0 %d\n", maxID+1)
	xref.WriteString("0000000000 65535 f \n")
	for id := 1; id <= maxID; id++ {
		fmt.Fprintf(&xref, "%010d 00000 n \n", offsets[id])
	}
	b.WriteString(xref.String())
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root %d 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		maxID+1, catalogID, xrefStart)

	return []byte(b.String())
}
