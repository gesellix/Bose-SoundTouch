package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

const (
	FormatTarGz = "tar.gz"
	FormatZip   = "zip"
)

var outputFlags = []cli.Flag{
	&cli.StringFlag{
		Name:    "output",
		Aliases: []string{"o"},
		Usage:   "Output archive file (default: soundtouch-backup-YYYY-MM-DD.tar.gz)",
		EnvVars: []string{"SOUNDTOUCH_BACKUP_OUTPUT"},
	},
	&cli.StringFlag{
		Name:  "format",
		Usage: "Archive format: tar.gz or zip",
		Value: FormatTarGz,
	},
}

func resolveOutputPath(output, format string) string {
	date := time.Now().Format("2006-01-02")

	ext := ".tar.gz"
	if format == FormatZip {
		ext = ".zip"
	}

	filename := "soundtouch-backup-" + date + ext

	if output == "" {
		return filename
	}

	if info, err := os.Stat(output); err == nil && info.IsDir() {
		return output + string(os.PathSeparator) + filename
	}

	return output
}

func archiveRoot() string {
	return "soundtouch-backup-" + time.Now().Format("2006-01-02")
}

func writeArchive(outputPath, format string, files map[string][]byte) error {
	if format == FormatZip {
		return writeZip(outputPath, files)
	}

	return writeTarGz(outputPath, files)
}

func writeTarGz(outputPath string, files map[string][]byte) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()

	tw := tar.NewWriter(gz)
	defer tw.Close()

	now := time.Now()
	for name, data := range files {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0644,
			Size:     int64(len(data)),
			ModTime:  now,
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("tar header %s: %w", name, err)
		}

		if _, err := tw.Write(data); err != nil {
			return fmt.Errorf("tar write %s: %w", name, err)
		}
	}

	return nil
}

func writeZip(outputPath string, files map[string][]byte) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			return fmt.Errorf("zip entry %s: %w", name, err)
		}

		if _, err := w.Write(data); err != nil {
			return fmt.Errorf("zip write %s: %w", name, err)
		}
	}

	return nil
}

func promptCredentials(emailHint string) (email, password string, err error) {
	r := bufio.NewReader(os.Stdin)

	if emailHint != "" {
		email = emailHint
	} else {
		fmt.Print("Bose account email: ")

		email, err = r.ReadString('\n')
		if err != nil {
			return
		}

		email = strings.TrimSpace(email)
	}

	fmt.Print("Password: ")

	raw, termErr := term.ReadPassword(int(os.Stdin.Fd()))

	fmt.Println()

	if termErr != nil {
		err = fmt.Errorf("reading password: %w (tip: use --password flag or BOSE_PASSWORD env var)", termErr)
		return
	}

	password = string(raw)

	return
}

func sanitizeName(name string) string {
	r := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_",
		"*", "_", "?", "_", "\"", "_",
		"<", "_", ">", "_", "|", "_",
		" ", "_",
	)

	return r.Replace(name)
}

func printOK(msg string)   { fmt.Printf("  ✓ %s\n", msg) }
func printFail(msg string) { fmt.Printf("  ✗ %s\n", msg) }
func printWarn(msg string) { fmt.Printf("  ⚠ %s\n", msg) }
