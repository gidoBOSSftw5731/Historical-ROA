package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gidoBOSSftw5731/Historical-ROA/movefromoldtonew"
	pb "github.com/gidoBOSSftw5731/Historical-ROA/proto"
	"github.com/gidoBOSSftw5731/log"
	"github.com/jackc/pgx"
	"google.golang.org/protobuf/encoding/protojson"
)

// inputROA is a Struct with all the data from the json
// we do NOT store this directly.
// https://mholt.github.io/json-to-go/
type inputROA struct {
	Asn       string `json:"asn"`
	Prefix    string `json:"prefix"`
	MaxLength int    `json:"maxLength"`
	Ta        string `json:"ta"`
	ParseCIDR string
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
	//client   *datastore.Client
	db       *pgx.Conn
	stmtMap  = make(map[string]*pgx.PreparedStatement)
	queryMap = map[string]string{"55mincheck": `SELECT time FROM last_modified`,
		"asnonly": `SELECT asn, prefix, mask, maxlen, ta, inserttimes FROM roas_arr
	WHERE asn = $1`,
		"prefixonly": `SELECT asn, prefix, mask, maxlen, ta, inserttimes FROM roas_arr
	WHERE prefix = $1 AND mask = $2`,
		"prefixandasn": `SELECT asn, prefix, mask, maxlen, ta, inserttimes FROM roas_arr
		WHERE asn = $1 AND prefix = $2 AND mask = $3`,
		"updatearray": `UPDATE roas_arr
	SET inserttimes = array_append(inserttimes, $1)
	WHERE asn = $2 AND prefix = $3 AND maxlen = $4 AND ta = $5 AND mask = $6`,
		"insertarray": `INSERT INTO roas_arr(asn, prefix, maxlen, ta, mask, inserttimes)
	VALUES ($1, $2, $3, $4, $5, $6)`}
	dbpass, dbip string
)

const (
	roaURL    = "https://hosted-routinator.rarc.net/json"
	projectID = "historical-roas"
)

func defineSQLStatements() {

	for i, j := range queryMap {
		var err error
		stmtMap[i], err = db.Prepare(i, j)
		if err != nil {
			log.Fatalln(err)
		}
	}
}

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
	dbpass = os.Getenv("DB_PASSWORD")
	if dbpass == "" {
		dbpass = "datboifff"
	}

	dbip = os.Getenv("DB_ADDR")
	if dbip == "" {
		dbip = "/cloudsql/historical-roas:us-east1:history3"
	}

	db, err = pgx.Connect(pgx.ConnConfig{
		Host:     dbip,
		User:     "postgres",
		Password: dbpass,
		Database: "roas",
	})
	if err != nil {
		log.Fatalln(err)
	}

	// prepare statements
	defineSQLStatements()

	http.HandleFunc("/update", pullToDB)
	http.HandleFunc("/", mainPage)
	http.HandleFunc("/hsts", hsts)
	http.HandleFunc("/aaaaaaaaaaaaaaaa", movefromoldtonew.Main)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), nil))
}

func hsts(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("strict-transport-security", "max-age=2629800")
	// If the X-Forwarded-Proto was set upstream as HTTP, then the request came in without TLS.
	if r.Header.Get("X-Forwarded-Proto") == "http" || r.URL.Scheme != "https" {
		r.URL.Scheme = "https"
		http.Redirect(w, r, r.URL.String(), http.StatusMovedPermanently)
	}
}

func mainPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("strict-transport-security", "max-age=2629800")

	tmpl, err := template.ParseFiles("./index.html")
	if err != nil {
		log.Errorln(err)
		return
	}

	if r.Method != http.MethodPost {
		tmpl.Execute(w, nil)
		return
	}

	input := inputROA{
		Asn:       r.FormValue("asn"),
		Prefix:    r.FormValue("prefix"),
		ParseCIDR: r.FormValue("parsecidr"),
	}

	if input.ParseCIDR != "" {
		_, n, _ := net.ParseCIDR(input.Prefix)
		input.Prefix = n.String()
	}

	inputStore := convInToStored(input)

	var hasASN, hasPrefix bool
	if inputStore.Asn != "" {
		hasASN = true
	}

	if inputStore.Prefix != "" && inputStore.Subnet != 0 {
		hasPrefix = true
	}

	log.Traceln(input)

	var rows *pgx.Rows
	switch {
	case hasASN && !hasPrefix:
		rows, err = db.Query(stmtMap["asnonly"].SQL, inputStore.Asn)
	case !hasASN && hasPrefix:
		rows, err = db.Query(stmtMap["prefixonly"].SQL, inputStore.Prefix, inputStore.Subnet)
	case hasASN && hasPrefix:
		rows, err = db.Query(stmtMap["prefixandasn"].SQL, inputStore.Asn, inputStore.Prefix, inputStore.Subnet)
	}
	if err != nil {
		ErrorHandler(w, r, 500, "lookup fail", err)
		return
	}

	var resultsarr pb.ResultArr
	for rows.Next() {
		var results pb.ResultsFromDB
		// asn, prefix, mask, maxlen, ta, inserttimes
		var intime []time.Time
		err = rows.Scan(&results.ASN, &results.Prefix, &results.Mask, &results.Maxlen, &results.Ta, &intime)
		if err != nil {
			ErrorHandler(w, r, 500, "idk couldnt convert sql to obj", err)
			return
		}
		for _, i := range intime {
			results.Unixtimearr = append(results.Unixtimearr, (i.Unix()))
		}

		results.Fullprefix = fmt.Sprintf("%v/%v", results.Prefix, results.Mask)
		switch {
		case results.Maxlen != results.Mask:
			results.Fullprefixrange = fmt.Sprintf("%v/%v => %v",
				results.Prefix, results.Mask, results.Maxlen)
		case results.Maxlen == results.Mask:
			results.Fullprefixrange = fmt.Sprintf("%v/%v", results.Prefix, results.Mask)
		}

		resultsarr.Results = append(resultsarr.Results, &results)
	}
	fmt.Fprintln(w, protojson.Format(&resultsarr))

}

// convert input data into stored data
func convInToStored(i inputROA) storedROA {
	// shut up I know its not correct terminology
	ipandmask := strings.Split(i.Prefix, "/")

	var mask int
	// probably doesnt need error checking
	if len(ipandmask) == 2 {
		mask, _ = strconv.Atoi(ipandmask[1])
	}

	return storedROA{
		Asn:       i.Asn,
		Prefix:    ipandmask[0],
		MaxLength: i.MaxLength,
		Ta:        i.Ta,
		Subnet:    mask,
	}
}

func pullToDB(w http.ResponseWriter, r *http.Request) {
	db, err := pgx.Connect(pgx.ConnConfig{
		Host:     dbip,
		User:     "postgres",
		Password: dbpass,
		Database: "roas",
	})
	if err != nil {
		log.Fatalln(err)
	}

	// see if there has been an update within 55 mins
	var lastIn time.Time
	db.QueryRow(stmtMap["55mincheck"].SQL).Scan(&lastIn)
	if lastIn.Add(55 * time.Minute).After(time.Now()) {
		log.Traceln("Record added in last 55 mins")
		ErrorHandler(w, r, 401, "already done", nil)
		return
	}
	w.WriteHeader(200)
	fmt.Fprintln(w, "ok")
	http.Error(w, "can't hijack rw", 200)
	hj, _ := w.(http.Hijacker)
	conn, _, _ := hj.Hijack()
	conn.Close()
	log.Debugln("starting update")

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
	// yes I dont need to prepare this in advance, it just looks neater to all put it down here.
	ustmt, err := txn.Prepare("updatevals",
		stmtMap["updatearray"].SQL)
	if err != nil {
		log.Fatalf("failed to create cursor: %v", err)
	}
	istmt, err := txn.Prepare("insertvals",
		stmtMap["insertarray"].SQL)
	if err != nil {
		log.Fatalf("failed to create cursor: %v", err)
	}

	now := time.Now()
	//var debug uint
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
		ra, err := txn.Exec(ustmt.SQL, now, i.Asn, ipandmask[0], i.MaxLength, i.Ta, mask)
		if err != nil {
			log.Errorln(err)
			continue
		}
		if ra.RowsAffected() == 0 {
			log.Debugln("Insertting row: ", now, i.Asn, ipandmask[0], i.MaxLength, i.Ta, mask)
			// asn, prefix, maxlen, ta, mask, inserttimes
			_, err = txn.Exec(istmt.SQL, i.Asn, ipandmask[0], i.MaxLength,
				i.Ta, mask, []time.Time{now})
			if err != nil {
				log.Errorln(err)
				continue
			}
		}

		//go log.Traceln(debug)
		//debug++

	}

	// All data is pending in the transaction, commit the transaction.
	//_, err = stmt.Exec()
	//if err != nil {
	//		log.Fatalf("failed to commit downloaded data: %v", err)
	//	}

	// I am not bothering to pre-prepare this
	txn.Exec("UPDATE last_modified SET time = $1;", now)

	// idk I used this in legacy code using database/sql and this fixed it, it has no
	// performance penalty so I frankly dont care
	_, err = txn.Exec(";")
	if err != nil {
		log.Errorln(err)
	}

	if err := txn.Commit(); err != nil {
		log.Fatalf("failed to commit and close the transaction: %v", err)
	}

	db.Close()

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
; modified to save storage
create table roas_arr (
	asn text,
	prefix text,
	maxlen int,
	ta text,
	mask int,
	inserttimes TIMESTAMP WITHOUT TIME ZONE[]
);
create table last_modified (
	time TIMESTAMP WITHOUT TIME ZONE
);
create index idx_as on roas_arr (asn);
create index idx_prefix_mask on roas_arr (prefix, mask);
create index idx_prefix_mask_asn on roas_arr (prefix, mask, asn);
*/
