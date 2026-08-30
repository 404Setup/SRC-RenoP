/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package caddy

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// RunCLI parses interactive Caddy installation arguments and applies the configuration.
func RunCLI(args []string, input io.Reader, output io.Writer) error {
	flags := flag.NewFlagSet("install-caddy", flag.ContinueOnError)
	flags.SetOutput(output)
	hostnameFlag := flags.String("hostname", "", "public hostname served by Caddy")
	caddyfileFlag := flags.String("caddyfile", "", "explicit Caddyfile path")
	configFlag := flags.String("config", "", "explicit RenoP config.yaml path")
	binaryFlag := flags.String("caddy-binary", "", "explicit Caddy executable path")
	skipReloadFlag := flags.Bool("skip-reload", false, "write files without requiring or reloading Caddy")
	flags.Usage = func() {
		fmt.Fprintln(output, "Usage: renop --install-caddy [options]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}

	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 256), 4096)
	hostname := strings.TrimSpace(*hostnameFlag)
	if hostname == "" {
		fmt.Fprint(output, "Public hostname: ")
		value, err := scanAnswer(scanner)
		if err != nil {
			return err
		}
		hostname = value
	}
	if _, err := NormalizeHostname(hostname); err != nil {
		return err
	}

	caddyfile := strings.TrimSpace(*caddyfileFlag)
	if caddyfile == "" {
		candidates, err := DiscoverCaddyfiles("")
		if err != nil {
			return err
		}
		switch len(candidates) {
		case 0:
			return errors.New("no Caddyfile was found; pass --caddyfile with its path")
		case 1:
			caddyfile = candidates[0]
		default:
			fmt.Fprintln(output, "Multiple Caddyfiles were found:")
			for index, candidate := range candidates {
				fmt.Fprintf(output, "  %d) %s\n", index+1, candidate)
			}
			fmt.Fprintf(output, "Select Caddyfile [1-%d]: ", len(candidates))
			answer, err := scanAnswer(scanner)
			if err != nil {
				return err
			}
			selection, err := strconv.Atoi(answer)
			if err != nil || selection < 1 || selection > len(candidates) {
				return errors.New("Caddyfile selection is invalid")
			}
			caddyfile = candidates[selection-1]
		}
	}

	result, err := Install(Options{
		Hostname:      hostname,
		CaddyfilePath: caddyfile,
		ConfigPath:    strings.TrimSpace(*configFlag),
		CaddyBinary:   strings.TrimSpace(*binaryFlag),
		SkipReload:    *skipReloadFlag,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "Configured %s through Caddy at %s.\n", result.Hostname, result.CaddyfilePath)
	fmt.Fprintf(output, "RenoP now listens for the proxy at %s using %s.\n", result.Upstream, result.ConfigPath)
	if result.Reloaded {
		fmt.Fprintln(output, "Caddy reloaded successfully.")
	} else {
		fmt.Fprintln(output, "Caddy was not reloaded; validate and reload it before serving traffic.")
	}
	if result.RestartRequired {
		fmt.Fprintln(output, "Restart RenoP to apply its listener and public-domain settings.")
	}
	return nil
}

func scanAnswer(scanner *bufio.Scanner) (string, error) {
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("read input: %w", err)
		}
		return "", errors.New("input ended before a value was provided")
	}
	return strings.TrimSpace(scanner.Text()), nil
}
