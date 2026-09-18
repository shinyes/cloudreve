// Throwaway helper: prints the storage policy each entity of a file lives on.
//
//	go run ./tools/relocate-check -db <path-to-cloudreve.db> [-file <name>]
//
// Run from the repository root. Requires the sqlite driver already used by the
// project (modernc.org/sqlite, pure Go - no CGO).
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "", "path to cloudreve sqlite database")
	fileName := flag.String("file", "", "only report files whose name matches")
	showProps := flag.Bool("props", false, "also report whether each entity carries encryption metadata")
	flag.Parse()

	if *dbPath == "" {
		fmt.Println("usage: go run ./tools/relocate-check -db <db> [-file <name>]")
		os.Exit(1)
	}

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer db.Close()

	query := `SELECT f.name, e.id, e.storage_policy_entities, e.reference_count, e.size, e.recycle_options, e.source
	          FROM entities e JOIN file_entities fe ON fe.entity_id = e.id
	          JOIN files f ON f.id = fe.file_id`
	args := []any{}
	if *fileName != "" {
		query += " WHERE f.name = ?"
		args = append(args, *fileName)
	}
	query += " ORDER BY f.name, e.id"

	rows, err := db.Query(query, args...)
	if err != nil {
		fmt.Println("query:", err)
		os.Exit(1)
	}
	defer rows.Close()

	perPolicy := map[int]int{}
	total := 0
	for rows.Next() {
		var (
			name     string
			id       int
			policyID int
			refCount int
			size     int64
			props    sql.NullString
			source   string
		)
		if err := rows.Scan(&name, &id, &policyID, &refCount, &size, &props, &source); err != nil {
			fmt.Println("scan:", err)
			os.Exit(1)
		}

		line := fmt.Sprintf("  file=%-10s entity=%-4d policy=%-3d refs=%-2d size=%d", name, id, policyID, refCount, size)
		if *showProps {
			state := "encrypt_metadata=none"
			if props.Valid && strings.Contains(props.String, "encrypt_metadata") {
				state = "encrypt_metadata=present"
			}
			line += fmt.Sprintf(" %s source=%s", state, source)
		}
		fmt.Println(line)

		perPolicy[policyID]++
		total++
	}

	fmt.Printf("TOTAL entities=%d\n", total)
	for policyID, count := range perPolicy {
		fmt.Printf("POLICY %d => %d entities\n", policyID, count)
	}
}
