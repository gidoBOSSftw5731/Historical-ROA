package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	pb "github.com/gidoBOSSftw5731/Historical-ROA/proto"
	"github.com/gidoBOSSftw5731/log"
	"github.com/shomali11/util/xhashes"
	"google.golang.org/api/iterator"
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

type storedROAWithTime struct {
	Asn       string `json:"asn"`
	Prefix    string `json:"prefix"`
	MaxLength int    `json:"maxLength"`
	Ta        string `json:"ta"`
	Subnet    int
	Times     []time.Time
}

// google cloud credentials file
type Creds struct {
	AuthProviderX509CertURL string `json:"auth_provider_x509_cert_url"`
	AuthURI                 string `json:"auth_uri"`
	ClientEmail             string `json:"client_email"`
	ClientID                string `json:"client_id"`
	ClientX509CertURL       string `json:"client_x509_cert_url"`
	PrivateKey              string `json:"private_key"`
	PrivateKeyID            string `json:"private_key_id"`
	ProjectID               string `json:"project_id"`
	TokenURI                string `json:"token_uri"`
	Type                    string `json:"type"`
}

var (
	client *bigquery.Client
	gcreds Creds
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

	gcredsPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if gcredsPath == "" {
		gcredsPath = "./Historical-ROAs-02210e643954.json"
	}
	gc, err := ioutil.ReadFile(gcredsPath)
	if err != nil {
		log.Fatalln(err)
	}

	err = json.Unmarshal(gc, &gcreds)
	if err != nil {
		log.Fatalln(err)
	}

	// open bigquery connection
	client, err = bigquery.NewClient(context.Background(), gcreds.ProjectID)
	if err != nil {
		log.Fatalln(err)
	}

	http.HandleFunc("/update", pullToDB)
	http.HandleFunc("/", mainPage)
	http.HandleFunc("/hsts", hsts)
	//http.HandleFunc("/aaaaaaaaaaaaaaaa", movefromoldtonew.Main)
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
	ctx := context.Background()
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

	var query *bigquery.Query
	switch {
	case hasASN && !hasPrefix:
		query = client.Query(`SELECT asn, prefix, mask, maxlen, ta, inserttimes FROM historical-roas.historical.roas_arr
		WHERE asn = @asn`)

	case !hasASN && hasPrefix:
		query = client.Query(`SELECT asn, prefix, mask, maxlen, ta, inserttimes FROM historical-roas.historical.roas_arr
		WHERE prefix = @prefix AND mask = @mask`)
	case hasASN && hasPrefix:
		query = client.Query(`SELECT asn, prefix, mask, maxlen, ta, inserttimes FROM historical-roas.historical.roas_arr
		WHERE asn = @asn AND prefix = @prefix AND mask = @mask`)
	}
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "asn",
			Value: inputStore.Asn,
		},
		{
			Name:  "prefix",
			Value: inputStore.Prefix,
		},
		{
			Name:  "mask",
			Value: inputStore.Subnet,
		},
	}
	job, err := query.Run(ctx)
	if err != nil {
		ErrorHandler(w, r, 500, "Error with query", err)
		return
	}

	status, err := job.Wait(ctx)
	if err := status.Err(); err != nil {
		ErrorHandler(w, r, 500, "Error with query", err)
		return
	}

	it, err := job.Read(ctx)
	if err != nil {
		ErrorHandler(w, r, 500, "Error with query", err)
		return
	}
	var resultsarr pb.ResultArr
	for {
		var row []bigquery.Value
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			ErrorHandler(w, r, 500, "Error with query", err)
			continue
		}
		var intime []time.Time
		var buf = row[5].([]bigquery.Value)

		for _, t := range buf {
			intime = append(intime, t.(time.Time))
		}

		var results = pb.ResultsFromDB{
			ASN:    row[0].(string),       // this
			Prefix: row[1].(string),       // is
			Mask:   int32(row[2].(int64)), // stupid
			Maxlen: int32(row[3].(int64)), // I hate you,
			Ta:     row[4].(string),       // Google
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

	// see if there has been an update within 55 mins
	query := client.Query("SELECT LAST_MODIFIED_TIME FROM INFORMATION_SCHEMA.SCHEMATA")
	row, _ := query.Read(context.Background())
	var time_row []bigquery.Value
	err := row.Next(&time_row)
	if err != nil {
		ErrorHandler(w, r, 500, "Cant get last edit time", err)
	}
	lastIn := time_row[0].(time.Time)

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

	inserter := client.Dataset("historical-roas:historical").Table("roas_arr").Inserter()

	var stored = make(map[string]struct{})

	currentQuery := client.Query(`SELECT asn, ta, prefix, mask, maxlen FROM historical-roas.historical.roas_arr`)
	ctx := context.Background()
	job, err := currentQuery.Run(ctx)
	if err != nil {
		ErrorHandler(w, r, 500, "Error with query", err)
		return
	}

	status, err := job.Wait(ctx)
	if err := status.Err(); err != nil {
		ErrorHandler(w, r, 500, "Error with query", err)
		return
	}

	it, err := job.Read(ctx)
	if err != nil {
		ErrorHandler(w, r, 500, "Error with query", err)
		return
	}
	var schema bigquery.Schema

	for {
		var row []bigquery.Value
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			ErrorHandler(w, r, 500, "Error with query", err)
			continue
		}

		stored[xhashes.MD5(fmt.Sprint(pb.ResultsFromDB{
			ASN:    row[0].(string),
			Ta:     row[1].(string),
			Prefix: row[2].(string),
			Mask:   int32(row[3].(int64)), // google, you are
			Maxlen: int32(row[4].(int64)), // disgusting
		}))] = struct{}{}

		if schema == nil {

			schema, err = bigquery.InferSchema(storedROAWithTime{})
			if err != nil {
				log.Errorln(err)
				schema = nil
			}
		}
	}

	now := []time.Time{time.Now()}
	var id int
	//var in []storedROA
	for _, i := range origIn.Roas {
		id++
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
		ctx := context.Background()

		switch _, ok := stored[xhashes.MD5(fmt.Sprint(pb.ResultsFromDB{
			ASN:    i.Asn,
			Ta:     i.Ta,
			Prefix: ipandmask[0],
			Mask:   int32(mask),
			Maxlen: int32(i.MaxLength),
		}))]; ok {
		case true:
			log.Traceln("Updating row: ", now, i.Asn, ipandmask[0], i.MaxLength, i.Ta, mask)
			// is already stored
			q := client.Query(`UPDATE historical-roas.historical.roas_arr
			SET inserttimes = ARRAY_CONCAT(inserttimes, @now) WHERE
			asn = @asn AND
			ta = @ta AND
			prefix = @prefix AND
			mask = @mask AND
			maxlen = @maxlen`)
			q.Parameters = []bigquery.QueryParameter{
				{
					Name:  "asn",
					Value: i.Asn,
				},
				{
					Name:  "prefix",
					Value: ipandmask[0],
				},
				{
					Name:  "mask",
					Value: mask,
				},
				{
					Name:  "ta",
					Value: i.Ta,
				}, {
					Name:  "maxlen",
					Value: i.MaxLength,
				},
				{
					Name:  "now",
					Value: now,
				},
			}
			job, err := q.Run(ctx)
			if err != nil {
				ErrorHandler(w, r, 500, "Error with query", err)
				continue
			}

			status, _ := job.Wait(ctx)
			if err := status.Err(); err != nil {
				ErrorHandler(w, r, 500, "Error with query", err)
				continue
			}
		case false:
			log.Debugln("Insertting row: ", now, i.Asn, ipandmask[0], i.MaxLength, i.Ta, mask)
			// asn, prefix, maxlen, ta, mask, inserttimes
			inserter.Put(ctx, bigquery.StructSaver{
				Schema: schema,
				Struct: &storedROAWithTime{
					Asn:       i.Asn,
					Ta:        i.Ta,
					Prefix:    ipandmask[0],
					Subnet:    mask,
					MaxLength: i.MaxLength,
					Times:     now,
				},
				InsertID: strconv.Itoa(id),
			})
			if err != nil {
				log.Errorln(err)
				continue
			}
		}

		//go log.Traceln(debug)
		//debug++

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
