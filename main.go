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
	queryMap = map[string]string{"55mincheck": `SELECT inserttime FROM roas 
	ORDER BY inserttime DESC LIMIT 1`,
		"asnonly": `SELECT asn, prefix, mask, maxlen, ta, inserttime FROM roas
	WHERE asn = $1`,
		"prefixonly": `SELECT asn, prefix, mask, maxlen, ta, inserttime FROM roas
	WHERE prefix = $1 AND mask = $2`,
		"prefixandasn": `SELECT asn, prefix, mask, maxlen, ta, inserttime FROM roas
		WHERE asn = $1 AND prefix = $2 AND mask = $3`}
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

	var results pb.ResultArr

	for rows.Next() {
		var asn, prefix, ta string
		var maxlen, mask int
		var itime time.Time
		if err := rows.Scan(&asn, &prefix, &mask, &maxlen, &ta, &itime); err != nil {
			log.Errorln(err)
			continue
		}

		log.Traceln(asn, prefix, mask, maxlen, ta, itime.Unix())
		log.Traceln(results.Results)

		var fullprefixrange string
		//check if I need to make fullprefixrange or not
		switch {

		case maxlen != mask:
			fullprefixrange = fmt.Sprintf("%v/%v => %v", prefix, mask, maxlen)
		case maxlen == mask:
			fullprefixrange = fmt.Sprintf("%v/%v", prefix, mask)
		}

		results.Results = append(results.Results, &pb.ResultsFromDB{
			ASN:             asn,
			Prefix:          prefix,
			Mask:            int32(mask),
			Maxlen:          int32(maxlen),
			Ta:              ta,
			Unixtime:        itime.Unix(),
			Fullprefix:      fmt.Sprintf("%v/%v", prefix, mask),
			Fullprefixrange: fullprefixrange,
		})
	}

	fmt.Fprintln(w, protojson.Format(&results))

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
		return
	}

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
	stmt, err := txn.Prepare("insertvals",
		"INSERT INTO roas(asn, prefix, maxlen, ta, mask) VALUES ($1, $2, $3, $4, $5)")
	if err != nil {
		log.Fatalf("failed to create cursor: %v", err)
	}

	var debug uint
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
		txn.Exec(stmt.SQL, i.Asn, ipandmask[0], i.MaxLength, i.Ta, mask)

		go log.Traceln(debug)
		debug++

	}

	// All data is pending in the transaction, commit the transaction.
	//_, err = stmt.Exec()
	//if err != nil {
	//		log.Fatalf("failed to commit downloaded data: %v", err)
	//	}

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
create table roas (
	asn text,
	prefix text,
	maxlen int,
	ta text,
	mask int,
	inserttime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
create index idx_as on roas (asn);
create index idx_prefix_mask on roas (prefix, mask);
create index idx_prefix_mask_asn on roas (prefix, mask, asn);
*/
