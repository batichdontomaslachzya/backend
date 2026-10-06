package main

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(1)
	}

	url := os.Args[1]
	method := "GET"
	max := 5
	key := ""

	for i := 2; i < len(os.Args); i++ {
		if os.Args[i] == "--method" {
			i++
			method = os.Args[i]
		} else if os.Args[i] == "--max-attempts" {
			i++
			max, _ = strconv.Atoi(os.Args[i])
		} else if os.Args[i] == "--idempotency-key" {
			i++
			key = os.Args[i]
		}
	}

	ok := method != "POST" || key != ""

	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	c := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	for i := 1; i <= max; i++ {
		req, _ := http.NewRequest(method, url, nil)

		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}

		res, e := c.Do(req)

		retry := false
		ms := 0

		if e != nil {
			fmt.Println("attempt", i, "error", e)
			retry = true
		} else {
			io.Copy(io.Discard, res.Body)
			res.Body.Close()

			fmt.Println("attempt", i, "status", res.StatusCode)

			if res.StatusCode >= 200 && res.StatusCode <= 399 {
				fmt.Println("result success attempts", i)
				return
			}

			if res.StatusCode == 429 ||
				res.StatusCode == 500 ||
				res.StatusCode == 502 ||
				res.StatusCode == 503 ||
				res.StatusCode == 504 {
				retry = true
			}

			if x := res.Header.Get("Retry-After"); x != "" {
				n, e := strconv.Atoi(x)
				if e == nil {
					ms = n * 1000
				}
			}
		}

		if !retry || !ok || i == max {
			fmt.Println("result failure attempts", i)
			os.Exit(1)
		}

		if ms == 0 {
			n := 200 * (1 << i)
			if n > 2000 {
				n = 2000
			}
			ms = r.Intn(n + 1)
		}

		fmt.Println("sleep_ms", ms)
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}

	os.Exit(1)
}
