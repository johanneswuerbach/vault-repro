package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/hashicorp/vault/api"
	"golang.org/x/sync/errgroup"
)

func checkError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

const (
	secretWrites = 1000
)

// Src https://stackoverflow.com/a/31832326
var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func RandStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func main() {
	rand.Seed(time.Now().UnixNano())

	testSecret := fmt.Sprintf("test-%s", RandStringRunes(10))
	testSecretData := fmt.Sprintf("secret/data/%s", testSecret)
	testSecretMetadata := fmt.Sprintf("secret/metadata/%s", testSecret)

	config := api.DefaultConfig()
	config.Address = "http://127.0.0.1:8200"

	client, err := api.NewClient(config)
	checkError(err)
	client.SetToken("DEV_TOKEN")

	logical := client.Logical()
	ctx := context.Background()

	// Increase versions to be stored so deleting a secret fully becomes slow

	fmt.Println("Increase versions to be stored", secretWrites)
	_, err = logical.WriteWithContext(ctx, "secret/config", map[string]interface{}{
		"max_versions": fmt.Sprintf("%d", secretWrites),
	})
	checkError(err)
	secretConfig, err := logical.ReadWithContext(ctx, "secret/config")
	checkError(err)
	fmt.Println("Secret config", secretConfig)

	//
	for delay := 50; delay < 250; delay++ {
		fmt.Println("===== Testing with delay", delay)

		// Create a large test secret with many versions

		fmt.Println("Writing test secret", testSecret)

		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(10)

		for i := 0; i < secretWrites; i++ {
			g.Go(func() error {
				_, err := logical.WriteWithContext(gctx, testSecretData, map[string]interface{}{
					"data": map[string]string{
						"data": RandStringRunes(100),
					},
				})
				return err
			})
		}
		checkError(g.Wait())

		secret, err := logical.ReadWithContext(ctx, testSecretData)
		checkError(err)
		fmt.Println("Current secret", secret)

		// Attempt a DELETE, but cancelling it quickly breaks the secret

		fmt.Println("Issue a delete, but cancel it mid-way")
		tctx, cancel := context.WithTimeout(ctx, time.Duration(delay)*time.Millisecond)
		defer cancel()

		_, err = logical.DeleteWithContext(tctx, testSecretMetadata)
		fmt.Println("Expected delete to fail")
		fmt.Println(err)

		secret, err = logical.ReadWithContext(ctx, testSecretData)
		fmt.Println("Reads fail now")
		fmt.Println("err", err)
		fmt.Println("secret", secret)

		if err != nil {
			fmt.Println("Caused race!!")
			break
		}
	}
}
