// Command zipscope-preview controls the Explorer preview pane integration.
//
// The setting is per extension and opt-in: nothing is registered unless the user
// turns it on from the app's settings page. This command exposes the same
// operations so an installer or uninstaller can drive them without the GUI.
//
// Usage:
//
//	zipscope-preview status
//	zipscope-preview enable  zip|cbz
//	zipscope-preview disable zip|cbz
//	zipscope-preview disable-all
//
// "status" prints one line per extension and exits non-zero only on error, so it
// is safe to call from a script that only wants to read the state.
package main

import (
	"fmt"
	"os"

	"github.com/tonbiattack/zip-scope/internal/shellreg"
)

func usage() {
	fmt.Fprintln(os.Stderr, "usage: zipscope-preview status | enable <zip|cbz> | disable <zip|cbz> | disable-all")
}

func printStatus() error {
	fmt.Printf("%-6s %-9s %s\n", "EXT", "ZIPSCOPE", "REGISTERED HANDLER")
	for _, ext := range shellreg.Extensions() {
		cls, err := shellreg.HandlerCLSID(ext)
		if err != nil {
			return err
		}
		state := "no"
		if shellreg.IsOwnHandler(cls) {
			state = "yes"
		}
		shown := cls
		if shown == "" {
			shown = "-"
		}
		fmt.Printf("%-6s %-9s %s\n", ext, state, shown)
	}
	return nil
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "status":
		err = printStatus()

	case "enable", "disable":
		if len(os.Args) < 3 {
			usage()
			os.Exit(2)
		}
		ext, normErr := shellreg.Normalize(os.Args[2])
		if normErr != nil {
			fmt.Fprintln(os.Stderr, normErr)
			os.Exit(2)
		}
		on := os.Args[1] == "enable"
		if err = shellreg.SetEnabled(ext, on); err == nil {
			action := "disabled"
			if on {
				action = "enabled"
			}
			fmt.Printf("%s %s\n", ext, action)
		}

	case "disable-all":
		if err = shellreg.DisableAll(); err == nil {
			fmt.Println("all extensions restored")
		}

	default:
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
