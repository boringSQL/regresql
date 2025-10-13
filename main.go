/*
regresql - Run regression tests for your SQL queries, against PostgreSQL

Usage:

	regresql [command]

Available Commands:

	help        Help about any command
	init        Initialize regresql for use in your project
	list        list candidates SQL files
	test        Run regression tests for your SQL queries
	update      Creates or updates the expected output files
*/
package main

import "github.com/boringsql/regresql/cmd"

func main() {
	cmd.Execute()
}
