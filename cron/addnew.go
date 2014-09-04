package main

import (
	"database/sql"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

import (
	_ "github.com/go-sql-driver/mysql"
)

var (
	db *sql.DB = nil
)

type SkiaCommitAuthor struct {
        Name        string      `json:"name"`
	Time        string      `json:"time"`
}

type SkiaCommitEntry struct {
	Commit      string               `json:"commit"`
	Author      *SkiaCommitAuthor    `json:"author"`
        Message     string               `json:"message"`
}

type SkiaJSON struct {
	Log        []*SkiaCommitEntry `json:"log"`
	Next        string          `json:"next"`
}

func init() {
	req, err := http.NewRequest("GET", "http://metadata/computeMetadata/v1/instance/attributes/readwrite", nil)
	if err != nil {
		panic(err)
	}
	client := http.Client{}
	req.Header.Add("X-Google-Metadata-Request", "True")
	if resp, err := client.Do(req); err == nil {
		password, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}
		db, err = sql.Open("mysql", fmt.Sprintf("readwrite:%s@tcp(173.194.240.40:3306)/skia?parseTime=true", password))
		if err != nil {
			panic(err)
		}
	} else {
		panic(err)
	}
}

func getCommits(start string) ([]*SkiaCommitEntry, error) {
	urlTmp := "https://skia.googlesource.com/skia/+log/%s..%s?format=JSON"
	url := fmt.Sprintf(urlTmp, start, "HEAD")
	results := []*SkiaCommitEntry{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed creating req: %s\n", err)
	}
	for req != nil {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("Failed to retrieve Skia JSON starting with hash %s: %s\n", start, err)
		}
		defer resp.Body.Close()
		result := &SkiaJSON{
			Log: []*SkiaCommitEntry{},
		}
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		rawJSON := buf.Bytes()
		maybeStrip := bytes.IndexAny(rawJSON, "\n")
		if maybeStrip >= 0 {
			rawJSON = rawJSON[maybeStrip:]
		}
		err = json.Unmarshal(rawJSON, result)
		if err != nil {
			return nil, fmt.Errorf("Failed to parse Skia JSON: %s\n", err)
		}
		results = append(results, result.Log...)
		if len(result.Next) > 0 {
			url = fmt.Sprintf(urlTmp, start, result.Next)
			req, err = http.NewRequest("GET", url, nil)
			if err != nil {
				return nil, fmt.Errorf("Failed creating req: %s\n", err)
			}
			time.Sleep(3000 * time.Millisecond)
		} else {
			req = nil
		}
	}
	return results, nil
}

func main() {
	row, err := db.Query(`SELECT githash, gitnumber
	    FROM githash
	    WHERE ts=(SELECT MAX(ts) FROM githash)`)
	if err != nil {
		panic(err)
	}
	gitnumber := -1
	githash := ""
	for row.Next() {
		if err := row.Scan(&githash, &gitnumber); err != nil {
			panic(err)
		}
		break
	}
	if gitnumber < 0 || len(githash) != 40 {
		panic(fmt.Errorf("Latest records gitnumber %d hash %s\n", gitnumber, githash))
	}
	res, err := getCommits(githash)
	if err != nil {
		fmt.Errorf("Cannot get commits: %s\n", err)
		return
	}
	if len(res) == 0 {
		fmt.Printf("No new commits.\n")
		return
	}
	insert := "INSERT INTO githash(gitnumber,ts,githash,author,message) VALUES"
	values := []string{}
	for i := len(res) - 1; i >= 0; i-- {
		gitnumber++
		r := res[i]
		t, err := time.Parse("Mon Jan 02 15:04:05 2006 -0700", r.Author.Time)
		if err != nil {
			panic(err)
		}
		val := fmt.Sprintf(`(%d,"%s","%s","%s","%s")`, gitnumber, t.UTC().Format("2006-01-02 15:04:05"), r.Commit, strings.Replace(r.Author.Name, "\"", "'", -1), strings.Replace(strings.Split(r.Message, "\n")[0], "\"", "'", -1))
		values = append(values, val)
	}
	if _, err := db.Exec(fmt.Sprintf("%s%s", insert, strings.Join(values, ","))); err != nil {
		panic(err)
	}
	fmt.Printf("Added %d Records, up to %d.\n", len(values), gitnumber)
}
