package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"time"
)

type result struct {
	url  string
	data []byte
	err  error
}

func main() {
	os.Exit(run())
}

func run() int {
	showHelp := false
	timeoutSeconds := 15

	flag.BoolVar(
		&showHelp,
		"h",
		false,
		"показать справку",
	)

	flag.BoolVar(
		&showHelp,
		"help",
		false,
		"показать справку",
	)

	flag.IntVar(
		&timeoutSeconds,
		"t",
		15,
		"таймаут запросов в секундах",
	)

	flag.IntVar(
		&timeoutSeconds,
		"timeout",
		15,
		"таймаут запросов в секундах",
	)

	flag.Parse()

	if showHelp {
		printUsage()
		return 0
	}

	if timeoutSeconds <= 0 {
		fmt.Fprintln(
			os.Stderr,
			"Таймаут должен быть больше нуля",
		)
		return 1
	}

	urls := flag.Args()

	if len(urls) == 0 {
		fmt.Fprintln(os.Stderr, "Не переданы URL")
		printUsage()
		return 1
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(timeoutSeconds)*time.Second,
	)
	defer cancel()

	client := &http.Client{}

	results := make(chan result, len(urls))

	for _, url := range urls {
		go func(currentURL string) {
			results <- fetch(ctx, client, currentURL)
		}(url)
	}

	var lastErr error

	for i := 0; i < len(urls); i++ {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				fmt.Fprintln(os.Stderr, "Превышен таймаут")
				return 228
			}

			fmt.Fprintln(os.Stderr, "Запросы отменены")
			return 1

		case res := <-results:
			if res.err != nil {
				lastErr = res.err

				if ctx.Err() == context.DeadlineExceeded {
					fmt.Fprintln(os.Stderr, "Превышен таймаут")
					return 228
				}

				continue
			}

			cancel()

			_, err := os.Stdout.Write(res.data)
			if err != nil {
				fmt.Fprintln(
					os.Stderr,
					"Ошибка вывода:",
					err,
				)
				return 1
			}

			return 0
		}
	}

	if ctx.Err() == context.DeadlineExceeded {
		fmt.Fprintln(os.Stderr, "Превышен таймаут")
		return 228
	}

	fmt.Fprintln(
		os.Stderr,
		"Все запросы завершились ошибкой:",
		lastErr,
	)

	return 1
}

func fetch(
	ctx context.Context,
	client *http.Client,
	url string,
) result {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		url,
		nil,
	)
	if err != nil {
		return result{
			url: url,
			err: err,
		}
	}

	response, err := client.Do(request)
	if err != nil {
		return result{
			url: url,
			err: err,
		}
	}
	defer response.Body.Close()

	data, err := httputil.DumpResponse(response, true)
	if err != nil {
		return result{
			url: url,
			err: err,
		}
	}

	return result{
		url:  url,
		data: data,
		err:  nil,
	}
}

func printUsage() {
	fmt.Println("Использование:")
	fmt.Println("  hedgedcurl [флаги] URL [URL ...]")
	fmt.Println()
	fmt.Println("Флаги:")
	fmt.Println(
		"  -t, --timeout SECONDS   таймаут, по умолчанию 15 секунд",
	)
	fmt.Println(
		"  -h, --help              показать справку",
	)
}
