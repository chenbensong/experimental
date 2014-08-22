package main

import (
  "database/sql"
  "fmt"
  "io/ioutil"
  "log"
  "net/http"
  "os"
  "os/exec"
  "strconv"
  "strings"
  _ "github.com/go-sql-driver/mysql"
)

var (
  db *sql.DB = nil
  repo = "/home/default/skia"
  gitbin = "/usr/local/bin/git"
  gitnumber = "/home/default/depot_tools/git_number.py"
)

func main() {
  req, err := http.NewRequest("GET", "http://metadata/computeMetadata/v1/instance/attributes/readwrite", nil)
  if err != nil {
    log.Printf("ERROR: Cannot get metadata: %q", err)
  }
  client := http.Client{}
  req.Header.Add("X-Google-Metadata-Request", "True")
  if resp, e := client.Do(req); e == nil {
    password, e := ioutil.ReadAll(resp.Body)
    if e != nil {
      log.Printf("ERROR: Failed to read password from metadata server: %q\n", e)
    }
    db, err = sql.Open("mysql", fmt.Sprintf("readwrite:%s@tcp(173.194.240.40:3306)/skia?parseTime=true", password))
    if err != nil {
      fmt.Printf("ERROR: Failed to open connection to SQL server: %q\n", err)
      panic(err)
    }
  } else {
    fmt.Println("DB CONN PROBLEM")
  }
  rows, er := db.Query("SELECT MAX(gitnumber) FROM githash")
  if er != nil {
    log.Printf("ERROR: Failed to query max git number: %q\n", er)
    panic(er)
  }
  maxnum := 0
  for rows.Next() {
    var n sql.RawBytes
    if err := rows.Scan(&n); err != nil {
      log.Printf("Error: failed to fetch from database: %q\n", err)
      continue
    }
    maxnum, err = strconv.Atoi(string(n[:]))
    fmt.Printf("MAX gitnumber: %d\n", maxnum)
  }
  if maxnum == 0 {
    maxnum = -1
  }

  cwd, _ := os.Getwd()
  os.Chdir(repo)
  cmd := exec.Command(gitbin, "pull", "origin", "master")
  out, err := cmd.Output()
  if err != nil {
    fmt.Printf("ERROR: Cannot update git repo: %q\n", err)
    panic(err)
  }
  cmd = exec.Command(gitnumber)
  out, err = cmd.Output()
  log.Printf("git number output:%s\n", out)
  currnum := 0
  if out != nil && len(out) != 0 {
    currnum, err = strconv.Atoi(strings.TrimSpace(string(out[:])))
    log.Printf("Current GIT number: %d\n", currnum)
  } else {
    log.Printf("No results for git number! %q\n", err)
  }
  if currnum <= maxnum {
    fmt.Printf("Latest git number %d <= max in database %d, exiting.\n",
        currnum, maxnum)
    os.Exit(0)
  }
  records_to_add := currnum - maxnum
  insert := "INSERT INTO githash(gitnumber,ts,githash,author,message) VALUES"
  cmd = exec.Command(gitbin, "log", "--pretty='%H,%an,%ad,%s'", "--date=raw")
  out, err = cmd.Output()
  hashes := ""
  if out != nil {
    values := ""
    gitnumbers := ""
    lines := strings.Split(string(out[:]), "\n")
    for i := 0; i < len(lines); i++ {
      to_end := false
      fields := strings.SplitN(strings.Trim(lines[i], "'"), ",", 4)
      if len(fields) != 4 {
        fmt.Println("LEN NOT 4")
        continue
      }
      ts := fields[2][:strings.Index(fields[2], " ")]
      githash := fields[0]
      hashes += githash + " "
      if i > 0 && i % 5000 == 0 {
        args := strings.Split(strings.TrimSpace(hashes), " ")
        cmd = exec.Command(gitnumber, args...)
        gitout, giterr := cmd.Output()
        if giterr != nil {
          fmt.Printf("GIT NUMBER ERR %q\n%s\n", giterr, hashes)
        }
        numbers := strings.Split(strings.TrimSpace(string(gitout)), "\n")
        for j := 0; j < len(numbers); j++ {
          num, numerr := strconv.Atoi(numbers[j])
          if numerr == nil && num == maxnum {
            to_end = true
            hashes = ""
            break
          }
          gitnumbers += numbers[j] + " "
        }
        hashes = ""
      }
      author := fields[1]
      message := strings.Replace(strings.Replace(fields[3], "\"", "'", -1), "|",
          "/", -1)
      values += fmt.Sprintf("FROM_UNIXTIME(%s),\"%s\",\"%s\",\"%s\"),|", ts, githash, author,
          message)
      if to_end {
        break
      }
    }
    if len(hashes) > 0 {
      args := strings.Split(strings.TrimSpace(hashes), " ")
      cmd = exec.Command(gitnumber, args...)
      gitout, giterr := cmd.Output()
      if giterr != nil {
        fmt.Printf("GIT NUMBER ERR2 %q\n%s\n", giterr, hashes)
      }
      numbers := strings.Split(strings.TrimSpace(string(gitout)), "\n")
      for j := 0; j < len(numbers); j++ {
        num, numerr := strconv.Atoi(numbers[j])
        if numerr == nil && num == maxnum {
            break
        }
        gitnumbers += numbers[j] + " "
      }
    }
    numbers := strings.Split(strings.TrimSpace(gitnumbers), " ")
    val_list := strings.Split(values, "|")
    vals := ""
    for i := 0; i < len(numbers); i++ {
      vals += "(" + numbers[i] + "," + val_list[i]
      if i > 0 && i % 5000 == 0 {
        _, er = db.Exec(strings.TrimRight(insert + vals, ","))
        if er != nil {
          log.Printf("ERROR: Failed to insert new git records: %q\n", er)
        panic(er)
        } else {
          fmt.Printf("Added batch Records %d.\n", records_to_add)
        }
        vals = ""
      }
    }
    if len(vals) > 0 {
      _, er = db.Exec(strings.TrimRight(insert + vals, ","))
      if er != nil {
        log.Printf("ERROR: Failed to insert new git records: %q\n", er)
      panic(er)
      } else {
        fmt.Printf("Added batch Records %d.\n", records_to_add)
      }
    }
  }
  os.Chdir(cwd)

}
