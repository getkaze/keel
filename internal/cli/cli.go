package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/getkaze/keel/internal/config"
	"github.com/getkaze/keel/internal/docker"
	"github.com/getkaze/keel/internal/model"
	"github.com/getkaze/keel/internal/updater"
)

// Run is the CLI entry point. args is os.Args[1:] starting from the subcommand.
func Run(args []string, keelDir, version string, _ ...[]byte) {
	if len(args) == 0 {
		printUsage(version)
		os.Exit(1)
	}

	switch args[0] {
	case "target":
		runTarget(args[1:], keelDir)
	case "start":
		runStart(args[1:], keelDir)
	case "stop":
		runStop(args[1:], keelDir)
	case "reset":
		runReset(args[1:], keelDir)
	case "dev":
		runDev(args[1:], keelDir)
	case "seed":
		runSeed(args[1:], keelDir)
	case "purge":
		runPurge(keelDir)
	case "hosts":
		runHosts(args[1:], keelDir)
	case "update":
		runUpdate(version)
	case "version":
		fmt.Println(version)
	case "help", "--help", "-h":
		printUsage(version)
	default:
		fmt.Fprintf(os.Stderr, "keel: unknown command %q\n\n", args[0])
		printUsage(version)
		os.Exit(1)
	}
}

// --- target ---

func runTarget(args []string, keelDir string) {
	if len(args) == 0 {
		// Show current target
		target, err := config.ReadTargetConfig(keelDir)
		exitOnErr(err)
		fmt.Printf("target: %s (%s @ %s)\n", target.Name, target.Mode, target.Host)
		return
	}

	name := args[0]

	// Validate target exists in targets.json
	available, err := config.ListTargets(keelDir)
	exitOnErr(err)

	found := false
	for _, t := range available {
		if t == name {
			found = true
			break
		}
	}
	if !found {
		fatalf("target %q not found. available: %s", name, strings.Join(available, ", "))
	}

	if err := config.SetTarget(keelDir, name); err != nil {
		fatalf("set target: %v\n\nhint: try running with sudo", err)
	}
	fmt.Printf("target set to: %s\n", name)
}

// --- start ---

func runStart(args []string, keelDir string) {
	ctx := context.Background()

	target, err := config.ReadTargetConfig(keelDir)
	exitOnErr(err)

	store := config.NewServiceStore(keelDir)
	runner := NewRunner(target, keelDir)

	fmt.Printf("target: %s (%s)\n", target.Name, target.Mode)

	// Specific services or group: no full group ordering, no seeders
	if len(args) > 0 {
		services, err := resolveServicesOrGroups(store, args)
		exitOnErr(err)
		for _, svc := range services {
			if err := runner.StartOne(ctx, svc, keelDir); err != nil {
				fmt.Fprintf(os.Stderr, "[%s] error: %v\n", svc.Name, err)
			}
		}
		return
	}

	// Start all: group ordering + seeders
	services, err := resolveServicesOrGroups(store, nil)
	exitOnErr(err)

	var infra, rest []model.Service
	for _, svc := range services {
		if svc.Group == "infra" {
			infra = append(infra, svc)
		} else {
			rest = append(rest, svc)
		}
	}

	// 1. Start infra
	for _, svc := range infra {
		if err := runner.StartOne(ctx, svc, keelDir); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] error: %v\n", svc.Name, err)
		}
	}

	// 2. Run seeders (if any)
	seeders := config.NewSeederStore(keelDir)
	executor := docker.NewSeederExecutorWithCmd(keelDir, store, seeders, runner)
	if executor.HasSeeders() {
		fmt.Println("--- running seeders ---")
		out := make(chan string, 64)
		errc := make(chan error, 1)
		go func() {
			defer close(out)
			defer close(errc)
			errc <- executor.RunAll(ctx, out)
		}()
		for line := range out {
			fmt.Println(line)
		}
		if err := <-errc; err != nil {
			fatalf("seeder failed: %v", err)
		}
	}

	// 3. Start remaining services
	for _, svc := range rest {
		if err := runner.StartOne(ctx, svc, keelDir); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] error: %v\n", svc.Name, err)
		}
	}
}

// --- stop ---

func runStop(args []string, keelDir string) {
	ctx := context.Background()

	target, err := config.ReadTargetConfig(keelDir)
	exitOnErr(err)

	store := config.NewServiceStore(keelDir)
	runner := NewRunner(target, keelDir)

	services, err := resolveServicesOrGroups(store, args)
	exitOnErr(err)

	fmt.Printf("target: %s (%s)\n", target.Name, target.Mode)
	for _, svc := range services {
		if err := runner.StopOne(ctx, svc); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] error: %v\n", svc.Name, err)
		}
	}
}

// --- dev ---

func runDev(args []string, keelDir string) {
	if len(args) < 2 {
		fatalf("usage: keel dev <service> <local-path>")
	}

	serviceName := args[0]
	localPath := args[1]

	// Resolve to absolute path.
	absPath, err := filepath.Abs(localPath)
	exitOnErr(err)

	if _, err := os.Stat(absPath); err != nil {
		fatalf("local path not found: %s", absPath)
	}

	ctx := context.Background()

	target, err := config.ReadTargetConfig(keelDir)
	exitOnErr(err)

	store := config.NewServiceStore(keelDir)
	runner := NewRunner(target, keelDir)

	svc, err := store.Get(serviceName)
	exitOnErr(err)
	if svc == nil {
		fatalf("service not found: %s", serviceName)
	}

	fmt.Printf("target: %s (%s)\n", target.Name, target.Mode)
	if err := runner.DevOne(ctx, *svc, absPath); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] dev error: %v\n", svc.Name, err)
	}

	// Restore the normal container after dev mode exits.
	fmt.Printf("\n[%s] restoring normal container...\n", svc.Name)
	restoreCtx := context.Background()
	if err := runner.StartOne(restoreCtx, *svc, keelDir); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] restore error: %v\n", svc.Name, err)
		os.Exit(1)
	}
}

// --- reset ---

func runReset(args []string, keelDir string) {
	if len(args) == 0 {
		fatalf("usage: keel reset <service> [service...] | --all")
	}

	ctx := context.Background()

	target, err := config.ReadTargetConfig(keelDir)
	exitOnErr(err)

	store := config.NewServiceStore(keelDir)
	runner := NewRunner(target, keelDir)

	var names []string
	if args[0] == "--all" {
		names = nil // nil = all services
	} else {
		names = args
	}

	services, err := resolveServicesOrGroups(store, names)
	exitOnErr(err)

	fmt.Printf("target: %s (%s)\n", target.Name, target.Mode)

	for _, svc := range services {
		fmt.Printf("[%s] stopping\n", svc.Name)
		_ = runner.Exec(ctx, "stop", svc.Hostname)

		fmt.Printf("[%s] removing\n", svc.Name)
		_ = runner.Exec(ctx, "rm", "-f", svc.Hostname)

		for _, vol := range svc.Volumes {
			// Only remove named volumes (no path separator = Docker-managed)
			name := strings.SplitN(vol, ":", 2)[0]
			if !strings.Contains(name, "/") && !strings.HasPrefix(name, ".") {
				fmt.Printf("[%s] removing volume %s\n", svc.Name, name)
				_ = runner.Exec(ctx, "volume", "rm", name)
			}
		}

		// Clear seeder state for seeders targeting this service
		docker.ClearSeederStateForService(keelDir, svc.Name)

		fmt.Printf("[%s] booting\n", svc.Name)
		if err := runner.Boot(ctx, svc, keelDir); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] boot error: %v\n", svc.Name, err)
		}
	}
}

// --- purge ---

func runPurge(keelDir string) {
	if os.Getuid() != 0 {
		fatalf("purge requires root privileges\n\nhint: run with sudo")
	}
	fmt.Printf("This will remove all containers and delete %s.\n", keelDir)
	fmt.Print("Type \"yes\" to confirm: ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	if strings.TrimSpace(scanner.Text()) != "yes" {
		fmt.Println("aborted")
		return
	}

	ctx := context.Background()
	store := config.NewServiceStore(keelDir)

	// Remove all containers (best-effort, ignore errors).
	services, _ := store.List()
	for _, svc := range services {
		fmt.Printf("[%s] removing\n", svc.Name)
		c := exec.CommandContext(ctx, "docker", "rm", "-f", svc.Hostname)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		_ = c.Run()
	}

	// Remove the keel network (best-effort).
	network := "keel-net"
	if cfg, err := store.GlobalConfig(); err == nil && cfg != nil && cfg.Network != "" {
		network = cfg.Network
	}
	fmt.Printf("removing network %s\n", network)
	c := exec.CommandContext(ctx, "docker", "network", "rm", network)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	_ = c.Run()

	// Delete the keel data directory.
	fmt.Printf("deleting %s\n", keelDir)
	if err := os.RemoveAll(keelDir); err != nil {
		fatalf("delete %s: %v", keelDir, err)
	}

	fmt.Println("done")
}

// --- update ---

func runUpdate(current string) {
	fmt.Println("==> checking for updates...")

	result, err := updater.Check(current)
	if err != nil {
		fatalf("check failed: %v", err)
	}

	if !result.Available {
		fmt.Printf("  ✓ already up to date (%s)\n", current)
		return
	}

	fmt.Printf("==> new version available: %s (current: %s)\n", result.Latest, current)
	fmt.Printf("==> downloading keel %s...\n", result.Latest)

	tmpPath, err := updater.Download(result.Latest)
	if err != nil {
		fatalf("download failed: %v", err)
	}
	defer os.Remove(tmpPath)

	if err := updater.Replace(tmpPath); err != nil {
		fatalf("replace failed: %v\n\nhint: try running with sudo", err)
	}

	fmt.Printf("  ✓ updated to %s\n", result.Latest)
	fmt.Println("  restart keel to apply.")
}

// --- seed ---

func runSeed(args []string, keelDir string) {
	ctx := context.Background()

	target, err := config.ReadTargetConfig(keelDir)
	exitOnErr(err)

	store := config.NewServiceStore(keelDir)
	runner := NewRunner(target, keelDir)
	seeders := config.NewSeederStore(keelDir)
	executor := docker.NewSeederExecutorWithCmd(keelDir, store, seeders, runner)

	out := make(chan string, 64)
	errc := make(chan error, 1)

	if len(args) > 0 {
		// Run single seeder
		seeder, err := seeders.Get(args[0])
		exitOnErr(err)
		if seeder == nil {
			fatalf("seeder not found: %s", args[0])
		}

		go func() {
			defer close(out)
			defer close(errc)
			errc <- executor.RunOne(ctx, out, seeder)
		}()
	} else {
		// Run all seeders
		go func() {
			defer close(out)
			defer close(errc)
			errc <- executor.RunAll(ctx, out)
		}()
	}

	for line := range out {
		fmt.Println(line)
	}

	if err := <-errc; err != nil {
		fatalf("%v", err)
	}
	fmt.Println("done")
}

// --- helpers ---

// resolveServicesOrGroups resolves arguments that can be service names or group names.
// A group name expands to all services in that group (sorted by start_order).
func resolveServicesOrGroups(store *config.ServiceStore, names []string) ([]model.Service, error) {
	if len(names) == 0 {
		return store.List()
	}

	groups, err := store.Groups()
	if err != nil {
		return nil, err
	}
	groupSet := map[string]bool{}
	for _, g := range groups {
		groupSet[g] = true
	}

	var result []model.Service
	for _, name := range names {
		if groupSet[name] {
			svcs, err := store.ListByGroup(name)
			if err != nil {
				return nil, err
			}
			result = append(result, svcs...)
		} else {
			svc, err := store.Get(name)
			if err != nil {
				return nil, err
			}
			if svc == nil {
				return nil, fmt.Errorf("service or group not found: %s", name)
			}
			result = append(result, *svc)
		}
	}
	return result, nil
}

func exitOnErr(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

func printUsage(version string) {
	fmt.Printf(`keel %s

Usage:
  keel [flags]                          start the web dashboard (port 60000)
  keel <command> [args]                 run a CLI command

Commands:
  target                                    show the active target
  target <name>                             switch to a target (reads data/targets.json)
  start                                     start all services on the active target
  start <service|group> [...]               start specific services or groups (infra, app, tools)
  stop                                      stop all services on the active target
  stop <service|group> [...]                stop specific services or groups
  reset --all                               destroy and recreate all containers
  reset <service> [service...]              destroy and recreate specific containers
  purge                                     remove all containers and delete the data directory
  update                                    check for updates and install the latest version
  dev <service> <local-path>               run a service in dev mode with local code mounted
  seed                                      run all seeders
  seed <name>                               run a single seeder by name
  hosts setup [--ip <addr>]                 add service domains to /etc/hosts
  hosts remove                              remove keel entries from /etc/hosts
  version                                   print version and exit
  help                                      print this help and exit

Dashboard flags:
  -port <n>                                 HTTP port (default 60000)
  -bind <addr>                              bind address (default 127.0.0.1)
  -keel-dir <path>                      data directory (default: OS-dependent)
  -dev                                      serve web assets from filesystem (dev mode)

Environment:
  KEEL_DIR                              data directory override for CLI commands

Examples:
  keel                                  open the dashboard
  keel target                           show current target (e.g. local)
  keel target ec2                       switch to ec2
  keel start                            start all services
  keel start infra                      start all infra services
  keel start redis mysql                start redis and mysql only
  keel stop app                         stop all app services
  keel stop traefik                     stop traefik
  keel reset --all                      recreate all containers from services/*.json
  keel reset redis                      recreate only redis
  keel dev mchtracker ~/projects/mchtracker   run mchtracker with local code + hot reload
`, version)
}
