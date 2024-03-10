//go:build windows

package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var archs = []string{
	"amd64",
	"arm64",
}

func main() {
	{
		CODE_SIGN_PFX := os.Getenv("CODE_SIGN_PFX")
		data, err := base64.StdEncoding.DecodeString(CODE_SIGN_PFX)
		if err != nil {
			fmt.Println("Cannot parse CODE_SIGN_PFX:", err)
			os.Exit(1)
		}
		pfxName := filepath.Join("private", "LiterMC-CodeSign.pfx")
		if err := os.WriteFile(pfxName, data, 0666); err != nil {
			fmt.Println("Cannot save CODE_SIGN_PFX:", err)
			os.Exit(1)
		}
		cmd := exec.Command("certutil", "-f", "-p", os.Getenv("CODE_SIGN_PFX_PASSWORD"), "-importpfx", pfxName)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Println("Error when loading cert:", err)
			os.Exit(1)
		}
	}

	buildTag := os.Getenv("TAG")
	fmt.Println("Detected tag:", buildTag)

	signtool := searchSignTool()
	if signtool == "" {
		fmt.Println("No signtool was found")
		os.Exit(1)
	}
	fmt.Println("signtool path:", signtool)

	ldflags := fmt.Sprintf("-X 'github.com/LiterMC/go-openbmclapi/internal/build.BuildVersion=%s'", buildTag)

	for _, arch := range archs {
		outputName := filepath.Join("output", fmt.Sprintf("go-openbmclapi-windows-%s.exe", arch))
		fmt.Printf("Building %s ...\n", outputName)
		{
			cmd := exec.Command("go", "build", "-o", outputName, "-ldflags", ldflags)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Env = append(([]string)(nil), os.Environ()...)
			cmd.Env = append(cmd.Env, "GOARCH="+arch)
			fmt.Println("Running", cmd.String())
			if err := cmd.Run(); err != nil {
				fmt.Println("Error when building:", err)
				dbugCmd := exec.Command("go", "env")
				dbugCmd.Stdout = os.Stdout
				dbugCmd.Stderr = os.Stderr
				dbugCmd.Run()
				os.Exit(1)
			}
		}
		fmt.Printf("Signing %s ...\n", outputName)
		{
			cmd := exec.Command(signtool, "sign", "/sm",
				"/n", "LiterMC-CodeSign",
				// "/tr", "http://timestamp.apple.com/ts01", "/td", "SHA256",
				"/fd", "SHA256", outputName)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Println("Error when signing:", err)
				os.Exit(1)
			}
		}
	}
}

func searchSignTool() string {
	var (
		signtool string
		version  string
	)
	const baseDir = `C:\Program Files (x86)\Windows Kits\10\bin`
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		panic(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "10.") || !strings.HasSuffix(name, ".0") {
			continue
		}
		if name > version {
			st := filepath.Join(baseDir, name, "x64", "signtool.exe")
			if stat, e := os.Stat(st); e == nil && !stat.IsDir() {
				signtool = st
				version = name
			}
		}
	}
	return signtool
}
