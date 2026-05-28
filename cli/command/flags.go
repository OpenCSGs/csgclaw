package command

import (
	"flag"
	"strings"
)

// ParseFlexible parses flags that may appear before or after positional arguments.
func ParseFlexible(fs *flag.FlagSet, args []string) error {
	flagArgs, positional := splitFlagsFromArgs(fs, args)
	return fs.Parse(append(flagArgs, positional...))
}

func splitFlagsFromArgs(fs *flag.FlagSet, args []string) (flagArgs, positional []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			return flagArgs, positional
		}
		if strings.HasPrefix(a, "-") {
			flagArgs = append(flagArgs, a)
			if strings.Contains(a, "=") {
				continue
			}
			if i+1 < len(args) && flagTakesValue(fs, flagName(a)) && !strings.HasPrefix(args[i+1], "-") {
				flagArgs = append(flagArgs, args[i+1])
				i++
			}
			continue
		}
		positional = append(positional, a)
	}
	return flagArgs, positional
}

func flagName(arg string) string {
	name := strings.TrimLeft(arg, "-")
	if idx := strings.Index(name, "="); idx >= 0 {
		name = name[:idx]
	}
	return name
}

func flagTakesValue(fs *flag.FlagSet, name string) bool {
	if name == "" {
		return false
	}
	f := fs.Lookup(name)
	if f == nil {
		return true
	}
	if bv, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bv.IsBoolFlag() {
		return false
	}
	return true
}
