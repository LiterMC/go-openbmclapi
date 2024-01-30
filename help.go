/**
 * OpenBmclAPI (Golang Edition)
 * Copyright (C) 2023 Kevin Z <zyxkad@gmail.com>
 * All rights reserved
 *
 *  This program is free software: you can redistribute it and/or modify
 *  it under the terms of the GNU Affero General Public License as published
 *  by the Free Software Foundation, either version 3 of the License, or
 *  (at your option) any later version.
 *
 *  This program is distributed in the hope that it will be useful,
 *  but WITHOUT ANY WARRANTY; without even the implied warranty of
 *  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 *  GNU Affero General Public License for more details.
 *
 *  You should have received a copy of the GNU Affero General Public License
 *  along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package main

import (
	"fmt"
	"os"
)

func printHelp() {
	fmt.Printf("Usage for %s\n\n", os.Args[0])
	fmt.Println("Sub commands:")
	fmt.Println("  help")
	fmt.Println("  \t" + "Show this message")
	fmt.Println()
	fmt.Println("  main | serve | <empty>")
	fmt.Println("  \t" + "Execute the main program")
	fmt.Println()
	fmt.Println("  license")
	fmt.Println("  \t" + "Print the full program license")
	fmt.Println()
	fmt.Println("  version")
	fmt.Println("  \t" + "Print the program's version")
	fmt.Println()
	fmt.Println("  zip-cache [options ...]")
	fmt.Println("  \t" + "Compress the cache directory")
	fmt.Println()
	fmt.Println("    Options:")
	fmt.Println("      " + "verbose | v : Show compressing files")
	fmt.Println("      " + "all | a : Compress all files")
	fmt.Println("      " + "overwrite | o : Overwrite compressed file even if it exists")
	fmt.Println("      " + "keep | k : Keep uncompressed file")
	fmt.Println()
	fmt.Println("  unzip-cache [options ...]")
	fmt.Println("  \t" + "Decompress the cache directory")
	fmt.Println()
	fmt.Println("    Options:")
	fmt.Println("      " + "verbose | v : Show decompressing files")
	fmt.Println("      " + "overwrite | o : Overwrite uncompressed file even if it exists")
	fmt.Println("      " + "keep | k : Keep compressed file")
}
