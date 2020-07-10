package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"cloud.google.com/go/datastore"
	"github.com/gidoBOSSftw5731/log"
	"github.com/lib/pq"
)

// inputROA is a Struct with all the data from the json
// we do NOT store this directly.
// https://mholt.github.io/json-to-go/
type inputROA struct {
	Asn       string `json:"asn"`
	Prefix    string `json:"prefix"`
	MaxLength int    `json:"maxLength"`
	Ta        string `json:"ta"`
}

type inputROAArr struct {
	Roas []inputROA `json:"roas"`
}

// storedROAs is what we store, we simply trim the subnet
// from the input ROA and store it seperately.
type storedROA struct {
	Asn       string `json:"asn"`
	Prefix    string `json:"prefix"`
	MaxLength int    `json:"maxLength"`
	Ta        string `json:"ta"`
	Subnet    int
}

var (
	client *datastore.Client
	db     *sql.DB
)

const (
	roaURL    = "https://hosted-routinator.rarc.net/json"
	projectID = "historical-roas"
)

func main() {
	// enable logging
	log.SetCallDepth(4)
	// set http port
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
		log.Tracef("using default port: %v", port)
	}

	// open SQL connection
	var err error
	dbpass := os.Getenv("DB_PASSWORD")
	if dbpass == "" {
		dbpass = "datboifff"
	}
	db, err = sql.Open("postgres", fmt.Sprintf("user=%v password=%v dbname=roas host=%v port=%v",
		"postgres", dbpass, os.Getenv("DB_ADDR"), "5432"))
	if err != nil {
		log.Fatalln(err)
	} else if db.Ping() != nil {
		log.Fatalln(db.Ping())
	}

	http.HandleFunc("/update", pullToDB)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), nil))
}

func pullToDB(w http.ResponseWriter, r *http.Request) {
	origIn, err := downloadRARC()
	if err != nil {
		ErrorHandler(w, r, 500, "Error parsing JSON", err)
		return
	}

	// Create a DB Transaction, one atomic change with many rows inserted.
	txn, err := db.Begin()
	if err != nil {
		log.Fatalf("failed to create transation: %v", err)
	}

	// Create the cursor, which gets filled with the Exec statement inside the for loop.
	stmt, err := txn.Prepare(
		pq.CopyIn("roas", "asn", "prefix", "maxlen", "ta", "mask"))
	if err != nil {
		log.Fatalf("failed to create cursor: %v", err)
	}

	//var in []storedROA
	for _, i := range origIn.Roas {
		// shut up I know its not correct terminology
		ipandmask := strings.Split(i.Prefix, "/")
		// probably doesnt need error checking
		mask, _ := strconv.Atoi(ipandmask[1])

		/*in = append(in, storedROA{
			Asn:       i.Asn,
			Prefix:    ipandmask[0],
			MaxLength: i.MaxLength,
			Ta:        i.Ta,
			Subnet:    mask,
		})*/

		stmt.Exec(i.Asn, ipandmask[0], i.MaxLength, i.Ta, mask)

	}

	// All data is pending in the transaction, commit the transaction.
	_, err = stmt.Exec()
	if err != nil {
		log.Fatalf("failed to commit downloaded data: %v", err)
	}

	if err := txn.Commit(); err != nil {
		log.Fatalf("failed to commit and close the transaction: %v", err)
	}

}

func downloadRARC() (*inputROAArr, error) {
	var form inputROAArr

	resp, err := http.Get(roaURL)
	if err != nil {
		return &form, err
	}
	defer resp.Body.Close()

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	jsonIn := buf.String()

	err = json.Unmarshal([]byte(jsonIn), &form)
	if err != nil {
		return &form, err
	}

	return &form, nil
}

//ErrorHandler is a function to handle HTTP errors
//copied from imgsrvr, slightly different formatting
func ErrorHandler(resp http.ResponseWriter, req *http.Request, status int, alert string, err error) {
	log.Errorln(err)
	resp.WriteHeader(status)
	log.Error("artifical http error: ", status)
	fmt.Fprintf(resp, "You have found an error! This error is of type %v. Built in alert: \n'%v',\n Would you like a <a href='https://http.cat/%v'>cat</a> or a <a href='https://httpstatusdogs.com/%v'>dog?</a>",
		status, alert, status, status)
}

/*
create table roas (
	asn text,
	prefix text,
	maxlen int,
	ta text,
	mask int,
	inserttime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
*/
