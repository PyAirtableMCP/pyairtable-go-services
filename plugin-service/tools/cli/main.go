package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	CLIVersion = "1.0.0"
	CLIName    = "pyairtable-plugin"
)

var (
	cfgFile string
	verbose bool
	debug   bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   CLIName,
	Short: "PyAirtable Plugin Development CLI",
	Long: `A command-line tool for developing, testing, and publishing PyAirtable plugins.

This CLI provides tools to:
- Create new plugin projects from templates
- Build and compile plugins to WebAssembly
- Test plugins locally with the development server
- Validate plugin manifests and security
- Publish plugins to registries
- Manage plugin installations and configurations`,
	Version: CLIVersion,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.pyairtable-plugin.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "debug output")

	// Bind flags to viper
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	viper.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug"))

	// Add subcommands
	rootCmd.AddCommand(newCmd)
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(publishCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(devCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(registryCmd)
}

// initConfig reads in config file and ENV variables
func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		viper.AddConfigPath(home)
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
		viper.SetConfigName(".pyairtable-plugin")
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix("PYAIRTABLE_PLUGIN")

	// Set default values
	viper.SetDefault("registry.default", "https://plugins.pyairtable.com")
	viper.SetDefault("dev.port", 3000)
	viper.SetDefault("build.target", "wasm32-unknown-unknown")
	viper.SetDefault("build.optimize", true)

	if err := viper.ReadInConfig(); err == nil && verbose {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}

// newCmd represents the new command
var newCmd = &cobra.Command{
	Use:   "new <plugin-name>",
	Short: "Create a new plugin project",
	Long: `Create a new plugin project from a template.

Available templates:
- typescript: TypeScript plugin template
- go: Go plugin template (compiled to WASM)
- rust: Rust plugin template (compiled to WASM)
- formula: Formula function plugin template
- ui: UI component plugin template
- webhook: Webhook handler plugin template`,
	Args: cobra.ExactArgs(1),
	RunE: runNewCommand,
}

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the plugin",
	Long: `Build the plugin and compile it to WebAssembly.

This command will:
1. Validate the plugin manifest
2. Compile the source code to WASM
3. Optimize the WASM binary
4. Generate plugin metadata
5. Create a plugin package ready for installation`,
	RunE: runBuildCommand,
}

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Test the plugin",
	Long: `Run tests for the plugin.

This command will:
1. Start a local test environment
2. Load the plugin into the runtime
3. Execute test scenarios
4. Report test results and coverage`,
	RunE: runTestCommand,
}

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the plugin",
	Long: `Validate the plugin manifest, permissions, and security.

This command will:
1. Validate the plugin manifest syntax
2. Check permission declarations
3. Analyze the WASM binary for security issues
4. Verify resource limit compliance
5. Check API compatibility`,
	RunE: runValidateCommand,
}

var publishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Publish the plugin to a registry",
	Long: `Publish the plugin to a plugin registry.

This command will:
1. Build and validate the plugin
2. Sign the plugin with your developer key
3. Upload the plugin to the specified registry
4. Update registry metadata`,
	RunE: runPublishCommand,
}

var installCmd = &cobra.Command{
	Use:   "install <plugin-name>",
	Short: "Install a plugin locally for development",
	Long: `Install a plugin locally for development and testing.

This command will:
1. Download the plugin from a registry
2. Install it in the local development environment
3. Configure it for testing`,
	Args: cobra.ExactArgs(1),
	RunE: runInstallCommand,
}

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Start the development server",
	Long: `Start a local development server for testing plugins.

The development server provides:
- Plugin runtime environment
- Mock PyAirtable API
- Real-time plugin reloading
- Debug tools and logging
- Interactive testing interface`,
	RunE: runDevCommand,
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage CLI configuration",
	Long:  `Manage CLI configuration including registries, authentication, and build settings.`,
}

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Manage plugin registries",
	Long:  `Manage plugin registries for publishing and installing plugins.`,
}

func init() {
	// new command flags
	newCmd.Flags().StringP("template", "t", "typescript", "Plugin template (typescript, go, rust, formula, ui, webhook)")
	newCmd.Flags().StringP("author", "a", "", "Plugin author")
	newCmd.Flags().StringP("description", "d", "", "Plugin description")
	newCmd.Flags().StringP("license", "l", "MIT", "Plugin license")

	// build command flags
	buildCmd.Flags().BoolP("release", "r", false, "Build in release mode with optimizations")
	buildCmd.Flags().StringP("target", "t", "", "Build target (default: wasm32-unknown-unknown)")
	buildCmd.Flags().BoolP("minify", "m", true, "Minify the WASM output")
	buildCmd.Flags().StringP("output", "o", "", "Output directory")

	// test command flags
	testCmd.Flags().StringP("env", "e", "test", "Test environment")
	testCmd.Flags().BoolP("coverage", "c", false, "Generate coverage report")
	testCmd.Flags().StringP("filter", "f", "", "Filter tests by pattern")
	testCmd.Flags().BoolP("watch", "w", false, "Watch for changes and re-run tests")

	// validate command flags
	validateCmd.Flags().BoolP("strict", "s", false, "Enable strict validation mode")
	validateCmd.Flags().StringP("rules", "r", "", "Custom validation rules file")

	// publish command flags
	publishCmd.Flags().StringP("registry", "r", "", "Target registry (default: configured default)")
	publishCmd.Flags().StringP("tag", "t", "latest", "Plugin tag")
	publishCmd.Flags().BoolP("dry-run", "n", false, "Show what would be published without actually publishing")
	publishCmd.Flags().StringP("key", "k", "", "Signing key file")

	// install command flags
	installCmd.Flags().StringP("version", "v", "latest", "Plugin version to install")
	installCmd.Flags().StringP("registry", "r", "", "Source registry")
	installCmd.Flags().BoolP("dev", "d", false, "Install in development mode")

	// dev command flags
	devCmd.Flags().IntP("port", "p", 3000, "Development server port")
	devCmd.Flags().StringP("host", "h", "localhost", "Development server host")
	devCmd.Flags().BoolP("reload", "r", true, "Enable auto-reload on file changes")
	devCmd.Flags().StringP("plugin", "", "", "Specific plugin to serve")

	// config subcommands
	configCmd.AddCommand(&cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Args:  cobra.ExactArgs(2),
		RunE:  runConfigSetCommand,
	})

	configCmd.AddCommand(&cobra.Command{
		Use:   "get <key>",
		Short: "Get a configuration value",
		Args:  cobra.ExactArgs(1),
		RunE:  runConfigGetCommand,
	})

	configCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all configuration",
		RunE:  runConfigListCommand,
	})

	// registry subcommands
	registryCmd.AddCommand(&cobra.Command{
		Use:   "add <name> <url>",
		Short: "Add a plugin registry",
		Args:  cobra.ExactArgs(2),
		RunE:  runRegistryAddCommand,
	})

	registryCmd.AddCommand(&cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a plugin registry",
		Args:  cobra.ExactArgs(1),
		RunE:  runRegistryRemoveCommand,
	})

	registryCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List configured registries",
		RunE:  runRegistryListCommand,
	})

	registryCmd.AddCommand(&cobra.Command{
		Use:   "search <query>",
		Short: "Search for plugins in registries",
		Args:  cobra.ExactArgs(1),
		RunE:  runRegistrySearchCommand,
	})
}

// Command implementations
func runNewCommand(cmd *cobra.Command, args []string) error {
	pluginName := args[0]
	template, _ := cmd.Flags().GetString("template")
	author, _ := cmd.Flags().GetString("author")
	description, _ := cmd.Flags().GetString("description")
	license, _ := cmd.Flags().GetString("license")

	fmt.Printf("Creating new plugin '%s' with template '%s'\n", pluginName, template)

	// Create project directory
	projectDir := filepath.Join(".", pluginName)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return fmt.Errorf("failed to create project directory: %w", err)
	}

	// Generate plugin manifest
	if err := generateManifest(projectDir, pluginName, template, author, description, license); err != nil {
		return fmt.Errorf("failed to generate manifest: %w", err)
	}

	// Generate template files
	if err := generateTemplate(projectDir, template, pluginName); err != nil {
		return fmt.Errorf("failed to generate template: %w", err)
	}

	fmt.Printf("Plugin '%s' created successfully in %s\n", pluginName, projectDir)
	fmt.Println("\nNext steps:")
	fmt.Printf("  cd %s\n", pluginName)
	fmt.Println("  pyairtable-plugin build")
	fmt.Println("  pyairtable-plugin test")

	return nil
}

func runBuildCommand(cmd *cobra.Command, args []string) error {
	release, _ := cmd.Flags().GetBool("release")
	target, _ := cmd.Flags().GetString("target")
	minify, _ := cmd.Flags().GetBool("minify")
	output, _ := cmd.Flags().GetString("output")

	fmt.Println("Building plugin...")

	// Read plugin manifest
	manifest, err := readManifest(".")
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	// Build based on detected language/framework
	builder, err := detectBuilder(".")
	if err != nil {
		return fmt.Errorf("failed to detect builder: %w", err)
	}

	buildConfig := BuildConfig{
		Release: release,
		Target:  target,
		Minify:  minify,
		Output:  output,
	}

	if err := builder.Build(manifest, buildConfig); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	fmt.Println("Build completed successfully!")
	return nil
}

func runTestCommand(cmd *cobra.Command, args []string) error {
	env, _ := cmd.Flags().GetString("env")
	coverage, _ := cmd.Flags().GetBool("coverage")
	filter, _ := cmd.Flags().GetString("filter")
	watch, _ := cmd.Flags().GetBool("watch")

	fmt.Printf("Running tests in %s environment...\n", env)

	testConfig := TestConfig{
		Environment: env,
		Coverage:    coverage,
		Filter:      filter,
		Watch:       watch,
	}

	runner, err := NewTestRunner(testConfig)
	if err != nil {
		return fmt.Errorf("failed to create test runner: %w", err)
	}

	results, err := runner.Run()
	if err != nil {
		return fmt.Errorf("test execution failed: %w", err)
	}

	printTestResults(results)

	if !results.AllPassed {
		os.Exit(1)
	}

	return nil
}

func runValidateCommand(cmd *cobra.Command, args []string) error {
	strict, _ := cmd.Flags().GetBool("strict")
	rulesFile, _ := cmd.Flags().GetString("rules")

	fmt.Println("Validating plugin...")

	validator, err := NewPluginValidator(strict, rulesFile)
	if err != nil {
		return fmt.Errorf("failed to create validator: %w", err)
	}

	results, err := validator.Validate(".")
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	printValidationResults(results)

	if !results.IsValid {
		os.Exit(1)
	}

	return nil
}

func runPublishCommand(cmd *cobra.Command, args []string) error {
	registry, _ := cmd.Flags().GetString("registry")
	tag, _ := cmd.Flags().GetString("tag")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	keyFile, _ := cmd.Flags().GetString("key")

	if registry == "" {
		registry = viper.GetString("registry.default")
	}

	fmt.Printf("Publishing plugin to registry '%s' with tag '%s'\n", registry, tag)

	if dryRun {
		fmt.Println("DRY RUN: Plugin would be published with the following configuration:")
		return printPublishPreview(registry, tag)
	}

	publisher, err := NewPluginPublisher(registry, keyFile)
	if err != nil {
		return fmt.Errorf("failed to create publisher: %w", err)
	}

	result, err := publisher.Publish(".", tag)
	if err != nil {
		return fmt.Errorf("publish failed: %w", err)
	}

	fmt.Printf("Plugin published successfully!\n")
	fmt.Printf("Plugin ID: %s\n", result.PluginID)
	fmt.Printf("Version: %s\n", result.Version)
	fmt.Printf("Registry URL: %s\n", result.URL)

	return nil
}

func runInstallCommand(cmd *cobra.Command, args []string) error {
	pluginName := args[0]
	version, _ := cmd.Flags().GetString("version")
	registry, _ := cmd.Flags().GetString("registry")
	dev, _ := cmd.Flags().GetBool("dev")

	fmt.Printf("Installing plugin '%s' version '%s'\n", pluginName, version)

	installer, err := NewPluginInstaller(registry)
	if err != nil {
		return fmt.Errorf("failed to create installer: %w", err)
	}

	installConfig := InstallConfig{
		Version:       version,
		DevMode:       dev,
		InstallPath:   ".plugins",
	}

	result, err := installer.Install(pluginName, installConfig)
	if err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	fmt.Printf("Plugin installed successfully to %s\n", result.InstallPath)
	return nil
}

func runDevCommand(cmd *cobra.Command, args []string) error {
	port, _ := cmd.Flags().GetInt("port")
	host, _ := cmd.Flags().GetString("host")
	reload, _ := cmd.Flags().GetBool("reload")
	plugin, _ := cmd.Flags().GetString("plugin")

	fmt.Printf("Starting development server on %s:%d\n", host, port)

	server, err := NewDevServer(DevServerConfig{
		Host:       host,
		Port:       port,
		AutoReload: reload,
		PluginPath: plugin,
	})
	if err != nil {
		return fmt.Errorf("failed to create dev server: %w", err)
	}

	return server.Start()
}

func runConfigSetCommand(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	viper.Set(key, value)
	if err := viper.WriteConfig(); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Printf("Configuration '%s' set to '%s'\n", key, value)
	return nil
}

func runConfigGetCommand(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := viper.Get(key)

	if value == nil {
		fmt.Printf("Configuration '%s' is not set\n", key)
	} else {
		fmt.Printf("%s = %v\n", key, value)
	}

	return nil
}

func runConfigListCommand(cmd *cobra.Command, args []string) error {
	fmt.Println("Current configuration:")
	for key := range viper.AllSettings() {
		value := viper.Get(key)
		fmt.Printf("  %s = %v\n", key, value)
	}
	return nil
}

func runRegistryAddCommand(cmd *cobra.Command, args []string) error {
	name := args[0]
	url := args[1]

	// Add to configuration
	registries := viper.GetStringMap("registries")
	if registries == nil {
		registries = make(map[string]interface{})
	}
	registries[name] = url
	viper.Set("registries", registries)

	if err := viper.WriteConfig(); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Printf("Registry '%s' added with URL '%s'\n", name, url)
	return nil
}

func runRegistryRemoveCommand(cmd *cobra.Command, args []string) error {
	name := args[0]

	registries := viper.GetStringMap("registries")
	if registries == nil {
		return fmt.Errorf("no registries configured")
	}

	if _, exists := registries[name]; !exists {
		return fmt.Errorf("registry '%s' not found", name)
	}

	delete(registries, name)
	viper.Set("registries", registries)

	if err := viper.WriteConfig(); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Printf("Registry '%s' removed\n", name)
	return nil
}

func runRegistryListCommand(cmd *cobra.Command, args []string) error {
	registries := viper.GetStringMap("registries")
	
	if len(registries) == 0 {
		fmt.Println("No registries configured")
		return nil
	}

	fmt.Println("Configured registries:")
	for name, url := range registries {
		fmt.Printf("  %s: %v\n", name, url)
	}

	return nil
}

func runRegistrySearchCommand(cmd *cobra.Command, args []string) error {
	query := args[0]

	fmt.Printf("Searching for plugins matching '%s'...\n", query)

	searcher, err := NewRegistrySearcher()
	if err != nil {
		return fmt.Errorf("failed to create searcher: %w", err)
	}

	results, err := searcher.Search(query)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	printSearchResults(results)
	return nil
}

// Helper types and functions would be implemented in separate files
type BuildConfig struct {
	Release bool
	Target  string
	Minify  bool
	Output  string
}

type TestConfig struct {
	Environment string
	Coverage    bool
	Filter      string
	Watch       bool
}

type InstallConfig struct {
	Version     string
	DevMode     bool
	InstallPath string
}

type DevServerConfig struct {
	Host       string
	Port       int
	AutoReload bool
	PluginPath string
}

// Placeholder implementations - these would be in separate files
func generateManifest(dir, name, template, author, description, license string) error {
	// Implementation for generating plugin manifest
	return nil
}

func generateTemplate(dir, template, name string) error {
	// Implementation for generating template files
	return nil
}

func readManifest(dir string) (interface{}, error) {
	// Implementation for reading plugin manifest
	return nil, nil
}

func detectBuilder(dir string) (interface{}, error) {
	// Implementation for detecting build system
	return nil, nil
}

func NewTestRunner(config TestConfig) (interface{}, error) {
	// Implementation for test runner
	return nil, nil
}

func NewPluginValidator(strict bool, rulesFile string) (interface{}, error) {
	// Implementation for plugin validator
	return nil, nil
}

func NewPluginPublisher(registry, keyFile string) (interface{}, error) {
	// Implementation for plugin publisher
	return nil, nil
}

func NewPluginInstaller(registry string) (interface{}, error) {
	// Implementation for plugin installer
	return nil, nil
}

func NewDevServer(config DevServerConfig) (interface{}, error) {
	// Implementation for development server
	return nil, nil
}

func NewRegistrySearcher() (interface{}, error) {
	// Implementation for registry searcher
	return nil, nil
}

func printTestResults(results interface{}) {
	// Implementation for printing test results
}

func printValidationResults(results interface{}) {
	// Implementation for printing validation results
}

func printPublishPreview(registry, tag string) error {
	// Implementation for printing publish preview
	return nil
}

func printSearchResults(results interface{}) {
	// Implementation for printing search results
}