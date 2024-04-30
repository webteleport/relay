package cmd

import "os"

func Arg1(fallback string) string {
	if len(os.Args) < 2 {
		return fallback
	}
	return os.Args[1]
}
