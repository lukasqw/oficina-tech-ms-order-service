// Package bdd is the entry point for the Godog scenario suite.
//
// Run with:
//
//	docker compose -f docker-compose.e2e.yml up -d --build
//	go test ./bdd/... -count=1
//
// Environment variables (with defaults that match docker-compose.e2e.yml):
//
//	MS1_URL              http://localhost:8081
//	MS2_URL              http://localhost:8082
//	MS3_URL              http://localhost:8083
//	MP_MOCK_URL          http://localhost:9999
//	MP_WEBHOOK_SECRET    e2e-webhook-secret
//	ADMIN_EMAIL          admin@oficina.test
//	ADMIN_PASSWORD       admin1234
package bdd

import (
	"flag"
	"os"
	"testing"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"

	"oficina-tech/bdd/steps"
)

// fullIntegration is true when all external services are available.
// When false, @integration scenarios are skipped gracefully via godog.ErrSkip.
var fullIntegration = os.Getenv("BDD_FULL_INTEGRATION") == "true"

var godogOpts = godog.Options{
	Output: colors.Colored(os.Stdout),
	Format: "pretty",
	// Tags can be filtered by setting BDD_TAGS, e.g. BDD_TAGS="@happy".
	Tags: os.Getenv("BDD_TAGS"),
	// Stop after first failure to keep the test output focused.
	StopOnFailure: false,
	// Strict mode fails on undefined/pending steps.
	// Disabled when not in full-integration mode so a suite of all-skipped
	// scenarios does not exit non-zero.
	Strict: fullIntegration,
}

func init() {
	godog.BindCommandLineFlags("godog.", &godogOpts)
}

func TestFeatures(t *testing.T) {
	flag.Parse()
	opts := godogOpts
	opts.TestingT = t
	if len(opts.Paths) == 0 {
		opts.Paths = []string{"../features"}
	}

	suite := godog.TestSuite{
		Name:                 "oficina-tech-e2e",
		ScenarioInitializer:  steps.RegisterScenario,
		TestSuiteInitializer: steps.RegisterSuite,
		Options:              &opts,
	}

	if status := suite.Run(); status != 0 {
		t.Fatalf("godog suite returned status %d", status)
	}
}
