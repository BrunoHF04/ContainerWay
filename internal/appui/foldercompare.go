package appui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"containerway/internal/fsutil"
)

// buildFolderCompareReport compara listagens de dois painéis (nomes, tipo, tamanho e data).
func buildFolderCompareReport(leftTitle, rightTitle, leftPath, rightPath string, leftRows, rightRows []fsutil.DirEntry) string {
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
	for k, le := range leftMap {
		re, ok := rightMap[k]
		if !ok {
			onlyLeft = append(onlyLeft, formatCompareLine(le, true))
			continue
		}
		if le.IsDir != re.IsDir || le.Size != re.Size || !le.ModTime.Equal(re.ModTime) {
			mismatch = append(mismatch, fmtCompareMismatch(le, re))
		}
	}
	for k, re := range rightMap {
		if _, ok := leftMap[k]; !ok {
			onlyRight = append(onlyRight, formatCompareLine(re, false))
		}
	}
	var b strings.Builder
	b.WriteString("Comparar pastas\n")
	b.WriteString(time.Now().Format("2006-01-02 15:04:05"))
	b.WriteString("\n\n")
	b.WriteString("Esquerda: ")
	b.WriteString(leftTitle)
	b.WriteString(" — ")
	b.WriteString(filepath.ToSlash(leftPath))
	b.WriteString("\nDireita:  ")
	b.WriteString(rightTitle)
	b.WriteString(" — ")
	b.WriteString(filepath.ToSlash(rightPath))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Só à esquerda (%d)\n", len(onlyLeft)))
	if len(onlyLeft) == 0 {
		b.WriteString("  (nenhum)\n")
	} else {
		for _, line := range onlyLeft {
			b.WriteString("  • ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString(fmt.Sprintf("\nSó à direita (%d)\n", len(onlyRight)))
	if len(onlyRight) == 0 {
		b.WriteString("  (nenhum)\n")
	} else {
		for _, line := range onlyRight {
			b.WriteString("  • ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString(fmt.Sprintf("\nDiferentes (nome igual, metadados distintos) (%d)\n", len(mismatch)))
	if len(mismatch) == 0 {
		b.WriteString("  (nenhum)\n")
	} else {
		for _, line := range mismatch {
			b.WriteString("  • ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString("\nNota: comparação por nome (sem distinção de maiúsculas), tipo, tamanho e data de modificação.\n")
	return b.String()
}

func formatCompareLine(e fsutil.DirEntry, left bool) string {
	side := "local"
	if !left {
		side = "remoto"
	}
	kind := "arquivo"
	if e.IsDir {
		kind = "pasta"
	}
	return fmt.Sprintf("%s (%s) %s %s", e.Name, side, kind, e.ModTime.Format("2006-01-02 15:04"))
}

func fmtCompareMismatch(le, re fsutil.DirEntry) string {
	return fmt.Sprintf("%s | esq: %s %d B @ %s | dir: %s %d B @ %s",
		le.Name,
		dirTypeLabel(le.IsDir), le.Size, le.ModTime.Format("02/01 15:04"),
		dirTypeLabel(re.IsDir), re.Size, re.ModTime.Format("02/01 15:04"),
	)
}

func dirTypeLabel(isDir bool) string {
	if isDir {
		return "pasta"
	}
	return "arq"
}
