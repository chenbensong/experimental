// main compiles Skia library in Release mode and records the lib sizes in
// Graphite and MySQL for the given githash range.
package main

import (
	"bufio"
	"database/sql"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

import (
	_ "github.com/go-sql-driver/mysql"
)

var (
	db *sql.DB = nil
	conn *net.TCPConn = nil
	metaURL = "http://metadata/computeMetadata/v1/instance/attributes/readwrite"
	dbHost = "173.194.240.40"
	dbUser = "readwrite"
	dbName = "skia"
	namePrefix = "size."
	logFile *os.File = nil
)

var (
	repo = flag.String("repo", "/home/default/repo", "The repo to build.")
	gitRead = flag.String("git_read", "/home/default/skia", "Repo to read git.")
	endRev = flag.String("end_rev", "4cb8bd18d9449328f4d27f22ad4045ecf2aa06bd", "The oldest revision to check.")
	graphiteServer = flag.String("graphite_server", "23.236.55.44:2003", "Where is Graphite metrics ingestion server running.")
	startRev = flag.String("start_rev", "a723b576aed31a6eb2bdda6388e6bd779d04c6b0", "The most recent revision to check, then check backwards to end_rev.")
)

func init() {
	flag.Parse()
	// Connect to MySQL. First read password from metadata.
	req, err := http.NewRequest("GET", metaURL, nil)
	if err != nil {
		panic(err)
	}
	client := http.Client{}
	req.Header.Add("Metadata-Flavor", "Google")
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	password, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	db, err = sql.Open("mysql", fmt.Sprintf("readwrite:%s@tcp(173.194.240.40:3306)/skia?parseTime=true", password))
	if err != nil {
		panic(err)
	}

	// Creates Graphite connection.
	addr, _ := net.ResolveTCPAddr("tcp", *graphiteServer)
	conn, err = net.DialTCP("tcp", nil, addr)
	if err != nil {
		panic(err)
	}

	// Creates log file.
	logFile, err := os.OpenFile("/home/default/backfill.log", os.O_RDWR | os.O_CREATE | os.O_APPEND, 0666)
	if err != nil {
    		panic(err)
	}
	log.SetOutput(logFile)
}

func getFileSizes(h string) map[string]int {
	m := make(map[string]int)
	cwd, _ := os.Getwd()
	os.Chdir(*repo)
	cmd := exec.Command("git", "checkout", "-b", h, h)
	_, err := cmd.Output()
	if err != nil {
		log.Printf("Problem checking out branch %s: %v", h, err)
		return m
	}
	cmd = exec.Command("gclient", "sync")
	_, err = cmd.Output()
	if err != nil {
		log.Printf("Problem running gclient sync %s: %v", h, err)
		return m
	}
	cmd = exec.Command("make", "clean")
	_, err = cmd.Output()
	if err != nil {
		log.Printf("Problem make clean %s: %v", h, err)
		return m
	}
	cmd = exec.Command("make", "BUILDTYPE=Release")
	_, err = cmd.Output()
	if err != nil {
		log.Printf("Problem make build %s: %v", h, err)
		cmd = exec.Command("git", "checkout", "origin/master")
		_, err = cmd.Output()
		if err != nil {
			log.Printf("Problem checkout origin/master for %s: %v", h, err)
			return m
		}
		cmd = exec.Command("git", "branch", "-D", h) 
		_, err = cmd.Output()
		if err != nil {
			log.Printf("Problem delete branch %s: %v", h, err)
			return m
		}
		return m
	}
	files, _ := filepath.Glob(*repo + "/out/Release/libskia*.a")
	if len(files) == 0 {
		files, _ = filepath.Glob(*repo + "/out/libskia*.a")
	}
	for _, f := range files {
		fi, err := os.Stat(f)
		if err != nil {
			log.Printf("Cannot stat %s in %s", f, h)
			continue
		}
		m[filepath.Base(f)] = int(fi.Size())
	}
	cmd = exec.Command("git", "checkout", "origin/master")
	_, err = cmd.Output()
	if err != nil {
		log.Printf("Problem checkout origin/master for %s: %v", h, err)
		return m
	}
	cmd = exec.Command("git", "branch", "-D", h) 
	_, err = cmd.Output()
	if err != nil {
		log.Printf("Problem delete branch %s: %v", h, err)
		return m
	}
	os.Chdir(cwd)
	return m
}

func main() {
	flag.Parse()
	cmd := exec.Command("git", "pull")
	cmd.Dir = *gitRead
	cmd = exec.Command("git", "log", "--format=format:%H%x20%at")
	cmd.Dir = *gitRead
	b, err := cmd.Output()
	if err != nil {
		panic(err)
	}
	lines := strings.Split(string(b), "\n")
	start := false
	for _, line := range lines {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		h := parts[0]
		if h == *startRev {
			start = true
		}
		if !start {
			continue
		}
		ts, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			log.Printf("Cannot parse ts: %s.", parts[1])
			continue
		}
		log.Printf("Start working on %s - %d:", h, ts)
		m := getFileSizes(h)
		for k, v := range m {
			if _, err := db.Exec(`INSERT IGNORE INTO sizes(ts,file,size)
				 VALUES(?,?,?)`, time.Unix(ts, 0).Format("2006-01-02 15:04:05"), k[7:len(k)-2], v); err != nil {
				log.Printf("Insert failed: %s, %d, %d", k, v, ts)
			}
			w := bufio.NewWriter(conn)
			fmt.Fprintf(w, "%s%s %d %d\n", namePrefix, strings.Replace(k, ".", "_", -1), v, ts)
			w.Flush()
		}
		log.Printf("Wrote %d Records for %s.", len(m), h)
		if h == *endRev {
			break
		}
	}

	logFile.Close()
}
