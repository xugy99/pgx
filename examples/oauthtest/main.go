package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/pterm/pterm"
)

var (
	sql = flag.String("sql", "select version()", "SQL query to execute")
	url = flag.String("url", "postgres://tester@localhost:5432/upm", "Database URL")
)

func main() {
	var (
		err  error
		opts pgx.ParseConfigOptions
	)
	flag.Parse()
	if *url == "" {
		log.Fatal("missing url")
	}

	// connect to server
	opts.GetOAuthBearerToken = func(ctx context.Context) string {
		if et := os.Getenv("OAUTH_BEARER_TOKEN"); et != "" {
			return et
		}
		log.Println("using default fake token, you should expect to see login failure")
		return "whatever_token"
	}

	conn, err := pgx.ConnectWithOptions(context.Background(), *url, opts)
	if err != nil {
		log.Fatalf("Unable to connect PostgreSQL by OAUTHBEARER: %v\n", err)
	}
	defer conn.Close(context.Background())

	// run sql
	rows, err := conn.Query(context.Background(), *sql)
	if err != nil {
		log.Fatalf("Failed to execute SQL: %v\n", err)
	}
	defer rows.Close()

	// convert to pterm table and print the result
	data := pterm.TableData{{}}
	//   - header row
	fieldDescriptions := rows.FieldDescriptions()
	for _, fd := range fieldDescriptions {
		data[0] = append(data[0], string(fd.Name))
	}
	//   - data row
	for rows.Next() {
		if rows.Err() == pgx.ErrNoRows {
			break
		}
		var row []string

		columns, _ := rows.Values()
		for _, v := range columns {
			row = append(row, fmt.Sprintf("%v", v))
		}
		data = append(data, row)
	}
	pterm.DefaultTable.
		WithBoxed(true).
		WithHasHeader(true).
		WithHeaderRowSeparator("-").
		WithData(data).
		Render() // show it
}
