package completion

import "strings"

type specItem struct {
	token       string
	description string
}

var commandSpecs = map[string][]specItem{
	"git": {
		{"status", "working tree status"}, {"add", "stage changes"}, {"commit", "record changes"},
		{"switch", "switch branch"}, {"checkout", "switch branch or restore files"}, {"branch", "list or create branches"},
		{"pull", "fetch and integrate"}, {"push", "update remote"}, {"fetch", "download remote refs"},
		{"diff", "show changes"}, {"log", "show commit history"}, {"restore", "restore files"},
		{"stash", "stash changes"}, {"rebase", "reapply commits"}, {"merge", "join histories"},
		{"--help", "show help"}, {"--version", "show version"},
	},
	"go": {
		{"run", "compile and run"}, {"test", "run tests"}, {"build", "compile packages"},
		{"fmt", "format packages"}, {"vet", "report suspicious code"}, {"mod", "module maintenance"},
		{"get", "add dependencies"}, {"install", "compile and install"}, {"generate", "run generators"},
		{"work", "workspace maintenance"}, {"env", "show Go environment"}, {"version", "show version"},
	},
	"docker": {
		{"ps", "list containers"}, {"run", "run a container"}, {"exec", "run in a container"},
		{"build", "build an image"}, {"pull", "download an image"}, {"push", "upload an image"},
		{"images", "list images"}, {"logs", "show container logs"}, {"compose", "manage compose application"},
		{"stop", "stop containers"}, {"rm", "remove containers"}, {"inspect", "show object details"},
	},
	"kubectl": {
		{"get", "display resources"}, {"describe", "show resource details"}, {"logs", "print container logs"},
		{"apply", "apply configuration"}, {"delete", "delete resources"}, {"exec", "execute in a container"},
		{"config", "modify kubeconfig"}, {"rollout", "manage a rollout"}, {"port-forward", "forward local ports"},
		{"--namespace", "select namespace"}, {"--context", "select context"},
	},
	"npm": {
		{"run", "run a package script"}, {"test", "run tests"}, {"install", "install dependencies"},
		{"uninstall", "remove a dependency"}, {"update", "update packages"}, {"init", "create package.json"},
		{"publish", "publish a package"}, {"outdated", "check outdated packages"}, {"audit", "run security audit"},
	},
	"cargo": {
		{"run", "run the package"}, {"test", "run tests"}, {"build", "compile the package"},
		{"check", "type-check quickly"}, {"fmt", "format Rust code"}, {"clippy", "run lints"},
		{"add", "add a dependency"}, {"update", "update dependencies"}, {"doc", "build documentation"},
	},
	"brew": {
		{"install", "install a formula"}, {"uninstall", "remove a formula"}, {"update", "update Homebrew"},
		{"upgrade", "upgrade packages"}, {"search", "search formulae"}, {"info", "show package info"},
		{"services", "manage background services"}, {"list", "list installed packages"}, {"doctor", "check system"},
	},
}

func specSuggestions(input string) []Suggestion {
	trimmed := strings.TrimLeft(input, " \t")
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return nil
	}
	spec, ok := commandSpecs[parts[0]]
	if !ok {
		return nil
	}
	start := strings.LastIndexAny(input, " \t\n") + 1
	prefix := input[start:]
	items := make([]Suggestion, 0, 6)
	for _, candidate := range spec {
		if prefix != "" && !strings.HasPrefix(candidate.token, prefix) {
			continue
		}
		if candidate.token == prefix {
			continue
		}
		if strings.HasPrefix(prefix, "-") != strings.HasPrefix(candidate.token, "-") {
			continue
		}
		items = append(items, Suggestion{
			Value: input[:start] + candidate.token, Label: candidate.token,
			Description: candidate.description, Source: "spec",
		})
		if len(items) == 6 {
			break
		}
	}
	return items
}
