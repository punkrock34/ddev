package util

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	osexec "os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/ddev/ddev/pkg/exec"
	"github.com/ddev/ddev/pkg/globalconfig"
	"github.com/ddev/ddev/pkg/nodeps"
	"github.com/ddev/ddev/pkg/output"
	"github.com/jedib0t/go-pretty/v6/text"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Failed will print a red error message and exit with failure.
func Failed(format string, a ...interface{}) {
	format = ColorizeText(format, "red")
	if a != nil {
		// output.UserOut.Fatalf(format, a...)
		output.UserErr.Fatalf(format, a...)
		// output.UserOut.WithField("level", "fatal").Fatalf(format, a...)
	} else {
		output.UserErr.Fatal(format)
		// output.UserOut.WithField("level", "fatal").Fatal(format)
	}
}

// Error will print a red error message but will not exit.
func Error(format string, a ...interface{}) {
	format = ColorizeText(format, "red")
	if a != nil {
		output.UserErr.Errorf(format, a...)
	} else {
		output.UserErr.Error(format)
	}
}

// Warning will present the user with warning text.
func Warning(format string, a ...interface{}) {
	format = ColorizeText(format, "yellow")
	if a != nil {
		output.UserErr.Warnf(format, a...)
	} else {
		output.UserErr.Warn(format)
	}
}

// WarningWithColor allows specifying a color for the warning to make it more visible
func WarningWithColor(color string, format string, a ...interface{}) {
	format = ColorizeText(format, color)
	if a != nil {
		output.UserErr.Warnf(format, a...)
	} else {
		output.UserErr.Warn(format)
	}
}

// Success will indicate an operation succeeded with colored confirmation text.
func Success(format string, a ...interface{}) {
	format = ColorizeText(format, "green")
	if a != nil {
		output.UserOut.Infof(format, a...)
	} else {
		output.UserOut.Info(format)
	}
}

// Debug Output controlled by DDEV_DEBUG environment variable
func Debug(format string, a ...interface{}) {
	if globalconfig.DdevDebug {
		n := time.Now()
		s := fmt.Sprintf(format, a...)
		if !output.JSONOutput {
			output.UserOut.Debugf("%s %s", n.Format("2006-01-02T15:04:05.000"), s)
		} else {
			output.UserOut.Debugf(s)
		}
	}
}

// Verbose Output controlled by DDEV_VERBOSE environment variable
func Verbose(format string, a ...interface{}) {
	if globalconfig.DdevVerbose {
		n := time.Now()
		s := fmt.Sprintf(format, a...)
		if !output.JSONOutput {
			output.UserOut.Debugf("%s %s", n.Format("2006-01-02T15:04:05.000"), s)
		} else {
			output.UserOut.Debugf(s)
		}
	}
}

// ShowDots displays dots one per second until done gets true
func ShowDots() chan bool {
	done := make(chan bool)
	go func() {
		for {
			select {
			case <-done:
				if !output.JSONOutput {
					_, _ = fmt.Fprintln(os.Stderr)
				}
				return
			default:
				if !output.JSONOutput {
					_, _ = fmt.Fprintf(os.Stderr, ".")
				}
				time.Sleep(1 * time.Second)
			}
		}
	}()
	return done
}

// FormatPlural is a simple wrapper which returns different strings based on the count value.
func FormatPlural(count int, single string, plural string) string {
	if count == 1 {
		return single
	}
	return plural
}

var letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// SetLetterBytes exists solely so that tests can override the default characters used by
// RandString. It should probably be avoided for 'normal' operations.
// this is actually used in utils_test.go (test only) so we set nolint on it.
func SetLetterBytes(lb string) {
	letterBytes = lb
}

// RandString returns a random string of given length n.
func RandString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

// HashSalt returns a hash of the projectName to be used as a salt.
// This is appropriate only for development work, but means
// that the HashSalt will be predictable for users on a team
func HashSalt(projectName string) string {
	hash := sha256.Sum256([]byte(projectName))
	return fmt.Sprintf("%x", hash)
}

// AskForConfirmation requests a y/n from user.
func AskForConfirmation() bool {
	response := GetInput("")
	okayResponses := []string{"y", "yes"}
	nokayResponses := []string{"n", "no", ""}
	responseLower := strings.ToLower(response)

	if nodeps.ArrayContainsString(okayResponses, responseLower) {
		return true
	} else if nodeps.ArrayContainsString(nokayResponses, responseLower) {
		return false
	}
	output.UserOut.Println("Please type yes or no and then press enter:")
	return AskForConfirmation()
}

// MapKeysToArray takes the keys of the map and turns them into a string array
func MapKeysToArray(mapWithKeys map[string]interface{}) []string {
	result := make([]string, 0, len(mapWithKeys))
	for v := range mapWithKeys {
		result = append(result, v)
	}
	return result
}

// GetContainerUIDGid returns the uid and gid (and string forms) to be used running most containers
func GetContainerUIDGid() (uidStr string, gidStr string, username string) {
	curUser, err := user.Current()
	if err != nil {
		Failed("Unable to determine username and related UID, etc. Please at least set $USER environment variable: %v", err)
	}
	uidStr = curUser.Uid
	gidStr = curUser.Gid
	username = curUser.Username
	// Remove at least spaces that aren't allowed in Linux usernames and can appear in Windows
	// Example problem usernames from https://stackoverflow.com/questions/64933879/docker-ddev-unicodedecodeerror-utf-8-codec-cant-decode-byte-0xe9-in-positio/64934264#64934264
	// "André Kraus", "Mück"
	// With docker-compose 1.29.2 you can't have a proper fully-qualified user pathname either
	// so end up with trouble based on that (not quoted correctly)
	// But for the context path it's possible to change the User home directory with
	// https://superuser.com/questions/890812/how-to-rename-the-user-folder-in-windows-10/1346983#1346983

	// Normalize username per https://stackoverflow.com/a/65981868/215713
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	username, _, _ = transform.String(t, username)

	username = strings.ReplaceAll(username, " ", "")
	username = strings.ToLower(username)
	username = strings.ReplaceAll(username, "(", "")
	username = strings.ReplaceAll(username, ")", "")

	// If we have a numeric username it's going to create havoc, so
	// change it into "a" + number
	// Example in https://github.com/ddev/ddev/issues/3187 - username="310822", uid=1663749668, gid=1240132652
	if !nodeps.IsLetter(string(username[0])) {
		username = "a" + username
	}

	// Windows usernames may have a \ to separate domain\user - get the user
	parts := strings.Split(username, `\`)
	username = parts[len(parts)-1]

	//// Windows userids are non-numeric,
	//// so we have to run as arbitrary user 1000. We may have a host uidStr/gidStr greater in other contexts,
	//// 1000 seems not to cause file permissions issues at least on docker-for-windows.
	if runtime.GOOS == "windows" {
		uidStr = "1000"
		gidStr = "1000"
	}
	return uidStr, gidStr, username
}

// IsCommandAvailable uses shell's "command" to find out if a command is available
// https://siongui.github.io/2018/03/16/go-check-if-command-exists/
// This lives here instead of in fileutil to avoid unnecessary import cycles.
func IsCommandAvailable(cmdName string) bool {
	_, err := osexec.LookPath(cmdName)
	if err == nil {
		return true
	}
	return false
}

// GetFirstWord returns the first space-separated word in a string.
func GetFirstWord(s string) string {
	arr := strings.Split(s, " ")
	return arr[0]
}

// FindBashPath returns the PATH to Bash on any system
// on Windows preferring git-bash
// On Windows we'll need the path to Bash to execute anything.
// Returns empty string if not found, path if found
func FindBashPath() string {
	if runtime.GOOS != "windows" {
		return "bash"
	}
	windowsBashPath, err := osexec.LookPath(`C:\Program Files\Git\bin\bash.exe`)
	if err != nil {
		// This one could come back with the WSL Bash, in which case we may have some trouble.
		windowsBashPath, err = osexec.LookPath("bash.exe")
		if err != nil {
			Warning("Not loading custom commands; Bash is not in PATH")
			return ""
		}
	}
	return windowsBashPath
}

// ElapsedTime is an easy way to report how long something took.
// It returns an anonymous function that, when called, will return the elapsed seconds.
func ElapsedTime(start time.Time) func() float64 {
	return func() float64 {
		elapsed := time.Since(start)
		return elapsed.Seconds()
	}
}

// ElapsedDuration is an easy way to report how long something took.
// It returns an anonymous function that, when called, will return the elapsed duration.
func ElapsedDuration(start time.Time) func() time.Duration {
	return func() time.Duration {
		return time.Since(start)
	}
}

// FormatDuration formats with 5m20s instead of lots of decimal points
// Based on https://stackoverflow.com/a/47342272/215713
func FormatDuration(d time.Duration) string {
	minutes := int(d.Minutes())
	seconds := int(math.Round(d.Seconds())) - (minutes * 60)
	if minutes == 0 {
		return fmt.Sprintf("%ds", seconds)
	}
	return fmt.Sprintf("%dm%ds", minutes, seconds)
}

// IsBeforeCutoffTime returns true if the current time is before
// the cutoff time, in format "01 Jan 21 10:00 UTC"
func IsBeforeCutoffTime(cutoff string) bool {
	cutoffTime, err := time.Parse(time.RFC822, cutoff)
	if err != nil {
		output.UserErr.Printf("Failed to parse cutoffTime %s: %v", cutoffTime, err)
	}
	currentTime := time.Now()
	if currentTime.Before(cutoffTime) {
		return true
	}
	return false
}

// ColorizeText colorizes text unless SimpleFormatting is turned on
func ColorizeText(s string, c string) (out string) {
	if !output.ColorsEnabled() || output.JSONOutput {
		text.DisableColors()
	}
	switch c {
	case "green":
		out = text.FgGreen.Sprint(s)
	case "magenta":
		out = text.FgMagenta.Sprint(s)
	case "red":
		out = text.FgRed.Sprint(s)
	case "yellow":
		out = text.FgYellow.Sprint(s)
	}
	return out
}

// Killall a process name on Linux/macOS/Windows.
// Avoid this as it may have unintended consequences.
func Killall(processName string) {
	if runtime.GOOS == "windows" {
		// Windows has a completely different process model, no SIGCHLD,
		// no killing of subprocesses. I wasn't successful in finding a way
		// to properly kill a process set using golang; rfay 20190622
		_, _ = exec.RunHostCommand("TASKKILL", "/T", "/F", processName)
	} else {
		_, _ = exec.RunHostCommand("killall", processName)
	}
}

// InterfaceSliceToStringSlice converts a slice of interfaces to a slice of strings
func InterfaceSliceToStringSlice(input interface{}) ([]string, error) {
	switch v := input.(type) {
	case []interface{}:
		raw := make([]string, len(v))
		for i := range v {
			if s, ok := v[i].(string); ok {
				raw[i] = s
			} else {
				return nil, fmt.Errorf("%v is not a string", v[i])
			}
		}
		return raw, nil
	default:
		return nil, fmt.Errorf("unexpected type: %T", input)
	}
}

// SliceToUniqueSlice processes a slice of string to make sure there are no duplicates
func SliceToUniqueSlice(inSlice *[]string) []string {
	mapStore := map[string]bool{}
	newSlice := []string{}

	for _, s := range *inSlice {
		// If we already found the value in our map, don't process into newSlice
		if _, ok := mapStore[s]; ok {
			continue
		}
		newSlice = append(newSlice, s)
		mapStore[s] = true
	}
	if len(newSlice) == 0 {
		return nil
	}
	return newSlice
}

// ArrayToReadableOutput generates a printable list of items in a readable way
func ArrayToReadableOutput(slice []string) (string, error) {
	if len(slice) == 0 {
		return "", fmt.Errorf("empty slice")
	}
	var b strings.Builder
	b.WriteString("\n")
	for _, item := range slice {
		b.WriteString("  - " + item + "\n")
	}
	return b.String(), nil
}

// WindowsPathToCygwinPath changes C:/path/to/something to //c/path/to/something
// This should only be used in CYGWIN/git-bash context
// Sadly, if we have a Windows drive name, it has to be converted from C:/ to //c for Win10Home/Docker toolbox
func WindowsPathToCygwinPath(windowsPath string) string {
	windowsPath = filepath.ToSlash(windowsPath)
	if len(windowsPath) >= 2 && string(windowsPath[1]) == ":" {
		drive := strings.ToLower(string(windowsPath[0]))
		windowsPath = "/" + drive + windowsPath[2:]
	}
	return windowsPath
}

// Chmod changes the file permissions of the named path,
// if the path already has the necessary permissions, do nothing.
// This is needed so that Mutagen doesn't track this change,
// don't use os.Chmod in the DDEV code, use util.Chmod instead.
func Chmod(path string, mode os.FileMode) error {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return err
	}
	// If the mode is the same, do nothing
	if fileInfo.Mode().Perm() == mode {
		return nil
	}
	return os.Chmod(path, mode)
}

// GetTimezone tries to find local timezone from the path, that can be
// either $TZ environment variable or /etc/localtime symlink
func GetTimezone(path string) (string, error) {
	// Use case-insensitive search for /zoneinfo/ in the file path.
	regex := regexp.MustCompile(`(?i)/.*?zoneinfo.*?/`)
	parts := regex.Split(strings.TrimSpace(path), 2)
	if len(parts) != 2 {
		// If this is not a path, but timezone, return it.
		_, err := time.LoadLocation(path)
		if err == nil {
			return path, nil
		}
		return "", fmt.Errorf("unable to read timezone from %s", path)
	}
	timezone := parts[1]
	// Remove leading prefixes if they exist.
	// https://stackoverflow.com/a/67888343/8097891
	for _, prefix := range []string{"posix/", "right/"} {
		timezone = strings.TrimPrefix(timezone, prefix)
	}
	if timezone == "" {
		return "", fmt.Errorf("unable to read timezone from %s", path)
	}
	_, err := time.LoadLocation(timezone)
	if err != nil {
		return "", fmt.Errorf("failed to load timezone '%s': %v", timezone, err)
	}
	return timezone, nil
}

// GetLocalTimezone tries to find local timezone from $TZ or /etc/localtime symlink
func GetLocalTimezone() (string, error) {
	timezone := ""
	if os.Getenv("TZ") != "" {
		timezone = os.Getenv("TZ")
	} else {
		localtimeFile := filepath.Join("/etc", "localtime")
		var err error
		timezone, err = filepath.EvalSymlinks(localtimeFile)
		if err != nil {
			return "", fmt.Errorf("unable to read timezone from %s file: %v", localtimeFile, err)
		}
	}
	return GetTimezone(timezone)
}

// SubtractSlices removes elements of slice b from slice a.
func SubtractSlices(a, b []string) []string {
	// Create a map to keep track of elements in slice b for quick lookup.
	bMap := make(map[string]bool)
	for _, elem := range b {
		bMap[elem] = true
	}

	// Collect elements from a that are not in b.
	var result []string
	for _, elem := range a {
		if _, found := bMap[elem]; !found {
			result = append(result, elem)
		}
	}

	return result
}

// ExpandHomedir expands the path to include the home directory if the path
// is prefixed with `~`. If it isn't prefixed with `~`, the path is
// returned as-is. Copied from archived
// https://github.com/mitchellh/go-homedir/blob/main/homedir.go
func ExpandHomedir(path string) (string, error) {
	if len(path) == 0 {
		return path, nil
	}

	if path[0] != '~' {
		return path, nil
	}

	if len(path) > 1 && path[1] != '/' && path[1] != '\\' {
		return "", errors.New("cannot expand user-specific home dir")
	}

	dir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, path[1:]), nil
}
