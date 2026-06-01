// Package foldercompare compara listagens de dois diretórios (nomes, tipo, tamanho, data).
package foldercompare

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"containerway/internal/fsutil"
)

// NamedEntry identifica um item na comparação (para acções na UI web).
type NamedEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
}

// Report texto legível da comparação entre dois painéis.
type Report struct {
	OnlyLeft       []string     `json:"onlyLeft"`
	OnlyRight      []string     `json:"onlyRight"`
	Mismatch       []string     `json:"mismatch"`
	OnlyLeftItems  []NamedEntry `json:"onlyLeftItems"`
	OnlyRightItems []NamedEntry `json:"onlyRightItems"`
	MismatchItems  []NamedEntry `json:"mismatchItems"`
	Text           string       `json:"text"`
}

// Build compara entradas de dois lados e devolve relatório.
func Build(leftTitle, rightTitle, leftPath, rightPath string, leftRows, rightRows []fsutil.DirEntry) Report {
	leftMap := map[string]fsutil.DirEntry{}
	rightMap := map[string]fsutil.DirEntry{}
	for _, e := range leftRows {
		if e.Name == "" || e.Name == ".." {
			continue
		}
		leftMap[strings.ToLower(e.Name)] = e
	}
	for _, e := range rightRows {
		if e.Name == "" || e.Name == ".." {
			continue
		}
		rightMap[strings.ToLower(e.Name)] = e
	}
	var onlyLeft, onlyRight, mismatch []string
	var onlyLeftItems, onlyRightItems, mismatchItems []NamedEntry
	for k, le := range leftMap {
		re, ok := rightMap[k]
		if !ok {
			onlyLeft = append(onlyLeft, formatLine(le, true))
			onlyLeftItems = append(onlyLeftItems, NamedEntry{Name: le.Name, IsDir: le.IsDir})
			continue
		}
		if le.IsDir != re.IsDir || le.Size != re.Size || !le.ModTime.Equal(re.ModTime) {
			mismatch = append(mismatch, fmtMismatch(le, re))
			mismatchItems = append(mismatchItems, NamedEntry{Name: le.Name, IsDir: le.IsDir || re.IsDir})
		}
	}
	for k, re := range rightMap {
		if _, ok := leftMap[k]; !ok {
			onlyRight = append(onlyRight, formatLine(re, false))
			onlyRightItems = append(onlyRightItems, NamedEntry{Name: re.Name, IsDir: re.IsDir})
		}
	}
	var b strings.Builder
	b.WriteString("Comparação de pastas\n")
	b.WriteString(time.Now().Format("2006-01-02 15:04:05"))
	b.WriteString("\n\n")
	b.WriteString("Esquerda (")
	b.WriteString(leftTitle)
	b.WriteString("): ")
	b.WriteString(filepath.ToSlash(leftPath))
	b.WriteString("\nDireita (")
	b.WriteString(rightTitle)
	b.WriteString("): ")
	b.WriteString(filepath.ToSlash(rightPath))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Só no painel esquerdo (%d):\n", len(onlyLeft)))
	if len(onlyLeft) == 0 {
		b.WriteString("  (nenhum)\n")
	} else {
		for _, line := range onlyLeft {
			b.WriteString("  • ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString(fmt.Sprintf("\nSó no painel direito (%d):\n", len(onlyRight)))
	if len(onlyRight) == 0 {
		b.WriteString("  (nenhum)\n")
	} else {
		for _, line := range onlyRight {
			b.WriteString("  • ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString(fmt.Sprintf("\nDiferenças (%d):\n", len(mismatch)))
	if len(mismatch) == 0 {
		b.WriteString("  (nenhum)\n")
	} else {
		for _, line := range mismatch {
			b.WriteString("  • ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString("\nNota: compara nomes, tipo (ficheiro/pasta), tamanho e data de modificação na listagem atual.\n")
	return Report{
		OnlyLeft: onlyLeft, OnlyRight: onlyRight, Mismatch: mismatch, Text: b.String(),
		OnlyLeftItems: onlyLeftItems, OnlyRightItems: onlyRightItems, MismatchItems: mismatchItems,
	}
}

func formatLine(e fsutil.DirEntry, left bool) string {
	side := "local"
	if !left {
		side = "remoto"
	}
	kind := "ficheiro"
	if e.IsDir {
		kind = "pasta"
	}
	return fmt.Sprintf("%s (%s) %s %s", e.Name, side, kind, e.ModTime.Format("2006-01-02 15:04"))
}

func fmtMismatch(le, re fsutil.DirEntry) string {
	return fmt.Sprintf("%s: %s %d %s | %s %d %s",
		le.Name,
		typeLabel(le.IsDir), le.Size, le.ModTime.Format("02/01 15:04"),
		typeLabel(re.IsDir), re.Size, re.ModTime.Format("02/01 15:04"),
	)
}

func typeLabel(isDir bool) string {
	if isDir {
		return "pasta"
	}
	return "ficheiro"
}
