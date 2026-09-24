package desktop

// ResolveLaunchOptionsForTest exposes desktop launch option normalization for tests.
func ResolveLaunchOptionsForTest(args []string, envBackground string) LaunchOptions {
	return ResolveLaunchOptions(args, envBackground)
}
