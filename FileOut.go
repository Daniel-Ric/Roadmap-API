// FileOut.go
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func main() {
	rootDir := flag.String("root", ".", "Projektwurzel (Startverzeichnis)")
	outName := flag.String("out", "projekt_zusammenfassung.txt", "Name der Ausgabedatei")
	onlyExt := flag.String("ext", "go,md,txt,json,yaml,yml,xml,html,css,js,ts,tsx,sql,sh,ps1,bat,toml,ini,env,proto,gradle,kt,rs,py,rb,php,pl,Makefile,Dockerfile", "Kommagetrennte Liste erlaubter Endungen/Namen")
	flag.Parse()

	allowed := buildAllowedSet(*onlyExt)

	excludedDirs := map[string]bool{
		".git":  true,
		".idea": true,
	}
	excludedFiles := map[string]bool{
		"go.mod": true,
	}

	out, err := os.Create(*outName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Fehler beim Erstellen der Ausgabedatei:", err)
		os.Exit(1)
	}
	defer out.Close()
	writer := bufio.NewWriter(out)
	defer writer.Flush()

	err = filepath.WalkDir(*rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Fprintln(os.Stderr, "Warnung, überspringe:", path, "->", err)
			return nil
		}
		// Verzeichnisse ggf. überspringen
		if d.IsDir() {
			if excludedDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		// Ausgabedatei selbst überspringen
		if filepath.Base(path) == filepath.Base(*outName) {
			return nil
		}
		// Einzelne Dateien ausschließen
		if excludedFiles[d.Name()] {
			return nil
		}
		// Nur erlaubte Textdateien
		if !isAllowedTextFile(path, d.Name(), allowed) {
			return nil
		}

		// Inhalt vorsichtig prüfen (kleiner Sniff, um Binärdaten rauszufiltern)
		f, err := os.Open(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Warnung, kann Datei nicht lesen:", path, "->", err)
			return nil
		}
		defer f.Close()

		const sniff = 8192
		buf := make([]byte, sniff)
		n, _ := f.Read(buf)
		snippet := buf[:n]

		// Kriterium: keine NUL-Bytes und (soweit möglich) gültiges UTF-8
		if hasNUL(snippet) || (n > 0 && !utf8.Valid(snippet)) {
			return nil
		}

		// zurück zum Anfang
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return nil
		}

		// Relativer Pfad für Header
		rel := path
		if r, err := filepath.Rel(*rootDir, path); err == nil {
			rel = r
		}

		// Header schreiben
		if _, err := fmt.Fprintf(writer, "--- %s ---\n", rel); err != nil {
			return err
		}

		// Inhalt kopieren
		if _, err := io.Copy(writer, f); err != nil {
			return err
		}

		// Leerzeile zwischen Dateien
		if _, err := fmt.Fprint(writer, "\n\n"); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		fmt.Fprintln(os.Stderr, "Fehler beim Durchlaufen:", err)
		os.Exit(1)
	}

	fmt.Println("Zusammenfassung erstellt:", *outName)
}

func buildAllowedSet(list string) map[string]bool {
	set := make(map[string]bool)
	for _, raw := range strings.Split(list, ",") {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		// sowohl mit Punkt als auch ohne erlauben
		if !strings.Contains(name, ".") && !strings.Contains(name, string(os.PathSeparator)) {
			set[strings.ToLower(name)] = true
			set["."+strings.ToLower(name)] = true
		} else {
			set[strings.ToLower(name)] = true
		}
	}
	return set
}

func isAllowedTextFile(fullPath, base string, allowed map[string]bool) bool {
	ext := strings.ToLower(filepath.Ext(base))
	baseLower := strings.ToLower(base)

	// Dockerfile/Makefile etc. (ohne Extension) erlauben
	if allowed[baseLower] {
		return true
	}
	// über Extension
	if allowed[ext] {
		return true
	}
	return false
}

func hasNUL(b []byte) bool {
	for _, c := range b {
		if c == 0x00 {
			return true
		}
	}
	return false
}
