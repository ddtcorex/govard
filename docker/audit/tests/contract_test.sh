#!/usr/bin/env bash
#
# Contract tests for the Govard-owned Magento lint runner.
#
# The suite drives docker/audit/bin/glint directly with
# stubbed PHP launchers, stubbed analyzers, and a stubbed Composer, so it needs
# only a PHP CLI and never a built image. When the host has no PHP CLI the
# suite re-executes itself inside a small public PHP container.
#
# Usage:
#   bash docker/audit/tests/contract_test.sh [case-name ...]
#
# Environment:
#   GOVARD_GLINT_TEST_PHP     PHP CLI used by the harness and stubs.
#   GOVARD_GLINT_TEST_IMAGE   Container image used when no PHP CLI exists.
#   GOVARD_GLINT_TEST_NO_DOCKER  Set to 1 to forbid the container fallback.

set -u

TESTS_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
CONTEXT_DIR=$(dirname -- "$TESTS_DIR")
RUNNER="$CONTEXT_DIR/bin/glint"
FIXTURE_STANDALONE="$TESTS_DIR/fixtures/standalone"

SUPPORTED_MATRIX="7.4 8.0 8.1 8.2 8.3 8.4 8.5"
AUTH_TOKEN="contract-secret-token-b3ad1f"

cases_run=0
cases_failed=0
case_failures=0
case_name=""

# ---------------------------------------------------------------------------
# Harness bootstrap
# ---------------------------------------------------------------------------

detect_php() {
    if [ -n "${GOVARD_GLINT_TEST_PHP:-}" ]; then
        printf '%s\n' "$GOVARD_GLINT_TEST_PHP"
        return 0
    fi
    local candidate
    for candidate in php php8.3 php8.2 php8.4 php8.1 php8.5 php7.4; do
        if command -v "$candidate" >/dev/null 2>&1; then
            printf '%s\n' "$candidate"
            return 0
        fi
    done
    return 1
}

reexec_in_container() {
    local image="${GOVARD_GLINT_TEST_IMAGE:-php:8.3-cli}"
    if [ "${GOVARD_GLINT_TEST_NO_DOCKER:-0}" = "1" ]; then
        printf 'contract tests need a PHP CLI; none found and the container fallback is disabled\n' >&2
        return 1
    fi
    if ! command -v docker >/dev/null 2>&1; then
        printf 'contract tests need a PHP CLI or Docker; neither is available\n' >&2
        return 1
    fi
    printf '# no host PHP CLI; running contract tests inside %s\n' "$image"
    docker run --rm \
        -v "$CONTEXT_DIR":/govard-context:ro \
        -e GOVARD_GLINT_TEST_INNER=1 \
        -e GOVARD_GLINT_TEST_PHP=php \
        "$image" \
        bash /govard-context/tests/contract_test.sh "$@"
}

# ---------------------------------------------------------------------------
# Assertions
# ---------------------------------------------------------------------------

fail() {
    case_failures=$((case_failures + 1))
    printf '  # %s\n' "$1"
}

assert_equals() {
    if [ "$2" = "$3" ]; then
        return 0
    fi
    fail "$1: expected [$2], got [$3]"
}

assert_contains() {
    case "$2" in
        *"$3"*) return 0 ;;
    esac
    fail "$1: [$3] not found"
}

assert_not_contains() {
    case "$2" in
        *"$3"*)
            fail "$1: unexpected [$3]"
            return 0
            ;;
    esac
}

assert_file_exists() {
    if [ -f "$2" ]; then
        return 0
    fi
    fail "$1: missing file $2"
}

assert_file_absent() {
    if [ ! -e "$2" ]; then
        return 0
    fi
    fail "$1: unexpected file $2"
}

json() {
    "$TEST_PHP" -r '
        $raw = @file_get_contents($argv[1]);
        if ($raw === false) { echo "<unreadable>"; exit(0); }
        $data = json_decode($raw, true);
        if ($data === null) { echo "<invalid-json>"; exit(0); }
        $value = $data;
        foreach (explode(".", $argv[2]) as $key) {
            if ($key === "") { continue; }
            if (is_array($value) && array_key_exists($key, $value)) {
                $value = $value[$key];
                continue;
            }
            echo "<missing>";
            exit(0);
        }
        if (is_bool($value)) { echo $value ? "true" : "false"; exit(0); }
        if (is_array($value)) { echo json_encode($value); exit(0); }
        echo (string) $value;
    ' "$1" "$2"
}

json_count() {
    "$TEST_PHP" -r '
        $raw = @file_get_contents($argv[1]);
        if ($raw === false) { echo "<unreadable>"; exit(0); }
        $data = json_decode($raw, true);
        if ($data === null) { echo "<invalid-json>"; exit(0); }
        $value = $data;
        foreach (explode(".", $argv[2]) as $key) {
            if ($key === "") { continue; }
            if (is_array($value) && array_key_exists($key, $value)) {
                $value = $value[$key];
                continue;
            }
            echo "<missing>";
            exit(0);
        }
        echo is_array($value) ? count($value) : "<not-a-list>";
    ' "$1" "$2"
}

# Collects one field from every php_results entry, in order.
json_php_field() {
    "$TEST_PHP" -r '
        $raw = @file_get_contents($argv[1]);
        if ($raw === false) { echo "<unreadable>"; exit(0); }
        $data = json_decode($raw, true);
        if (!is_array($data) || !isset($data["php_results"])) { echo "<missing>"; exit(0); }
        $parts = [];
        foreach ($data["php_results"] as $result) {
            $value = $result;
            foreach (explode(".", $argv[2]) as $key) {
                if (is_array($value) && array_key_exists($key, $value)) {
                    $value = $value[$key];
                    continue;
                }
                $value = "<missing>";
                break;
            }
            if (is_bool($value)) { $value = $value ? "true" : "false"; }
            if (is_array($value)) { $value = json_encode($value); }
            $parts[] = (string) $value;
        }
        echo implode(",", $parts);
    ' "$1" "$2"
}

# Collects "name:status" for every phase of one php result.
json_phases() {
    "$TEST_PHP" -r '
        $raw = @file_get_contents($argv[1]);
        if ($raw === false) { echo "<unreadable>"; exit(0); }
        $data = json_decode($raw, true);
        if (!is_array($data) || !isset($data["php_results"][(int) $argv[2]])) { echo "<missing>"; exit(0); }
        $parts = [];
        foreach ($data["php_results"][(int) $argv[2]]["phases"] as $phase) {
            $parts[] = $phase["name"] . ":" . $phase["status"];
        }
        echo implode(",", $parts);
    ' "$1" "$2"
}

# Collects "tool|rule" for every finding of every php result.
json_finding_tools() {
    "$TEST_PHP" -r '
        $raw = @file_get_contents($argv[1]);
        if ($raw === false) { echo "<unreadable>"; exit(0); }
        $data = json_decode($raw, true);
        if (!is_array($data) || !isset($data["php_results"])) { echo "<missing>"; exit(0); }
        $parts = [];
        foreach ($data["php_results"] as $result) {
            foreach ((array) $result["findings"] as $finding) {
                $parts[] = $finding["tool"] . "|" . (isset($finding["rule"]) ? $finding["rule"] : "");
            }
        }
        echo implode(",", $parts);
    ' "$1"
}

tree_digest() {
    (
        cd "$1" || exit 1
        find . -mindepth 1 \( -type f -o -type l -o -type d \) -printf '%y %m %p\n' | LC_ALL=C sort
        find . -type f -print0 | LC_ALL=C sort -z | xargs -0 -r cat
    ) | "$TEST_PHP" -r 'echo hash("sha256", stream_get_contents(STDIN));'
}

# ---------------------------------------------------------------------------
# Case fixtures and stubs
# ---------------------------------------------------------------------------

write_php_launchers() {
    local dir=$1 version
    for version in $SUPPORTED_MATRIX; do
        cat >"$dir/bin/php$version" <<STUB
#!/bin/sh
exec "$TEST_PHP" "\$@"
STUB
        chmod 0755 "$dir/bin/php$version"
    done
}

write_analyzer_stubs() {
    local dir=$1 version
    for version in $SUPPORTED_MATRIX; do
        mkdir -p "$dir/toolchains/php-$version/vendor/bin"
        # Only the 8.1+ toolchains ship the PHP compatibility standard, exactly
        # as the real image does: they resolve magento/magento-coding-standard
        # 40, which depends on magento/php-compatibility-fork. PHP 7.4 and 8.0
        # resolve coding-standard 4, which does not, so both must report a
        # skipped compat phase.
        case "$version" in
            7.4|8.0) ;;
            *)
                mkdir -p "$dir/toolchains/php-$version/vendor/magento/php-compatibility-fork/PHPCompatibility"
                printf '<ruleset name="PHPCompatibility"/>\n' \
                    >"$dir/toolchains/php-$version/vendor/magento/php-compatibility-fork/PHPCompatibility/ruleset.xml"
                ;;
        esac
        cat >"$dir/toolchains/php-$version/vendor/bin/phpcs" <<'STUB'
<?php
$standard = 'Magento2';
$testVersion = '';
$arguments = array_slice($argv, 1);
foreach ($arguments as $index => $argument) {
    if (strpos($argument, '--standard=') === 0) {
        $standard = substr($argument, strlen('--standard='));
    }
    if ($argument === 'testVersion' && isset($arguments[$index + 1])) {
        $testVersion = $arguments[$index + 1];
    }
}
$compatibilityPass = $standard === 'PHPCompatibility';
$mode = $compatibilityPass
    ? (getenv('GOVARD_TEST_PHPCOMPAT_MODE') ?: 'pass')
    : (getenv('GOVARD_TEST_PHPCS_MODE') ?: 'pass');
$sleep = (int) (getenv('GOVARD_TEST_SLEEP_SECONDS') ?: 0);
$reportFile = null;
$target = null;
$skipNext = 0;
foreach ($arguments as $argument) {
    if ($skipNext > 0) {
        $skipNext--;
        continue;
    }
    if ($argument === '--runtime-set') {
        $skipNext = 2;
        continue;
    }
    if (strpos($argument, '--report-file=') === 0) {
        $reportFile = substr($argument, strlen('--report-file='));
        continue;
    }
    if (strpos($argument, '-') !== 0) {
        $target = $argument;
    }
}
if ($testVersion === '') {
    fwrite(STDERR, "stub phpcs was not told which PHP version to test\n");
    exit(3);
}
if ($mode === 'sleep' && $sleep > 0) {
    sleep($sleep);
    $mode = 'pass';
}
$file = rtrim((string) $target, '/') . '/Model/Greeting.php';
$messages = [];
$exit = 0;
if ($mode === 'findings' && $compatibilityPass) {
    $messages[] = ['message' => 'Function create_function() is removed since PHP ' . $testVersion, 'source' => 'PHPCompatibility.FunctionUse.RemovedFunctions.create_functionDeprecatedRemoved', 'severity' => 5, 'type' => 'ERROR', 'line' => 7, 'column' => 9, 'fixable' => false];
    $exit = 1;
} elseif ($mode === 'findings') {
    $messages[] = ['message' => 'Line exceeds the configured limit.', 'source' => 'Generic.Files.LineLength.TooLong', 'severity' => 5, 'type' => 'ERROR', 'line' => 12, 'column' => 3, 'fixable' => false];
    $exit = 1;
}
if ($mode === 'internal') {
    $messages[] = ['message' => 'syntax error, unexpected token', 'source' => 'Internal.Exception', 'severity' => 5, 'type' => 'ERROR', 'line' => 1, 'column' => 1, 'fixable' => false];
    $exit = 1;
}
if ($mode === 'error') {
    fwrite(STDERR, "stub phpcs failed\n");
    exit(2);
}
$payload = ['totals' => ['errors' => count($messages), 'warnings' => 0, 'fixable' => 0], 'files' => []];
if ($messages !== []) {
    $payload['files'][$file] = ['errors' => count($messages), 'warnings' => 0, 'messages' => $messages];
}
if ($reportFile !== null) {
    file_put_contents($reportFile, json_encode($payload));
} else {
    echo json_encode($payload);
}
exit($exit);
STUB
        cat >"$dir/toolchains/php-$version/vendor/bin/phpstan" <<'STUB'
<?php
$mode = getenv('GOVARD_TEST_PHPSTAN_MODE') ?: 'pass';
$sleep = (int) (getenv('GOVARD_TEST_SLEEP_SECONDS') ?: 0);
$target = null;
foreach (array_slice($argv, 2) as $argument) {
    if (strpos($argument, '-') !== 0) {
        $target = $argument;
    }
}
if ($mode === 'sleep' && $sleep > 0) {
    sleep($sleep);
    $mode = 'pass';
}
if ($mode === 'invalid') {
    echo "PHP Fatal error: stub phpstan crashed\n";
    exit(255);
}
if ($mode === 'stale') {
    // First invocation simulates a stale resultCache referencing a deleted var/deployer file.
    // The marker lives under GOVARD_LINT_CACHE_DIR (shared across retries).
    $cacheDir = getenv('GOVARD_LINT_CACHE_DIR') ?: sys_get_temp_dir();
    $marker = rtrim($cacheDir, '/') . '/.phpstan_stale_first';
    if (!file_exists($marker)) {
        @file_put_contents($marker, "1");
        fwrite(STDERR, "PHP Warning:  hash_file(/source/var/deployer/releases/1/var/www/html/app/etc/env.php): Failed to open stream: No such file or directory in phar:///opt/govard/toolchains/php-8.4/vendor/phpstan/phpstan/phpstan.phar/src/Analyser/ResultCache/ResultCacheManager.php on line 1189\n");
        fwrite(STDERR, "In ResultCacheManager.php line 1191:\n  Could not read file: /source/var/deployer/releases/1/var/www/html/app/etc/env.php\n");
        exit(2);
    }
    // Second invocation (after glint purged the cache) succeeds.
}
$file = rtrim((string) $target, '/') . '/Model/Greeting.php';
$payload = ['totals' => ['errors' => 0, 'file_errors' => 0], 'files' => [], 'errors' => []];
$exit = 0;
if ($mode === 'findings') {
    $payload['files'][$file] = ['errors' => 1, 'messages' => [['message' => 'Parameter $subject has no value type specified.', 'line' => 20, 'ignorable' => true, 'identifier' => 'missingType.parameter']]];
    $payload['totals'] = ['errors' => 0, 'file_errors' => 1];
    $exit = 1;
}
if ($mode === 'compat') {
    $payload['errors'][] = 'Analysed code is not compatible with the configured PHP version.';
    $payload['totals'] = ['errors' => 1, 'file_errors' => 0];
    $exit = 1;
}
echo json_encode($payload);
exit($exit);
STUB
    done
}

write_composer_stub() {
    local dir=$1
    cat >"$dir/bin/composer" <<'STUB'
<?php
$mode = getenv('GOVARD_TEST_COMPOSER_MODE') ?: 'pass';
$workingDir = null;
foreach (array_slice($argv, 1) as $argument) {
    if (strpos($argument, '--working-dir=') === 0) {
        $workingDir = substr($argument, strlen('--working-dir='));
    }
}
if ($mode === 'error') {
    fwrite(STDERR, "stub composer failed\n");
    exit(1);
}
if ($workingDir !== null) {
    @mkdir($workingDir . '/vendor/composer', 0755, true);
    file_put_contents($workingDir . '/vendor/autoload.php', "<?php\nreturn true;\n");
}
echo "stub composer install completed\n";
exit(0);
STUB
}

new_case() {
    case_name=$1
    case_failures=0
    CASE_DIR="$WORK_ROOT/$case_name"
    rm -rf "$CASE_DIR"
    mkdir -p "$CASE_DIR"/bin "$CASE_DIR"/cache "$CASE_DIR"/output "$CASE_DIR"/home \
        "$CASE_DIR"/source "$CASE_DIR"/toolchains "$CASE_DIR"/auth
    cp -R "$FIXTURE_STANDALONE/." "$CASE_DIR/source/"
    write_php_launchers "$CASE_DIR"
    write_analyzer_stubs "$CASE_DIR"
    write_composer_stub "$CASE_DIR"
    printf '{"http-basic":{"repo.example.com":{"username":"contract","password":"%s"}}}\n' \
        "$AUTH_TOKEN" >"$CASE_DIR/auth/auth.json"
    chmod 0600 "$CASE_DIR/auth/auth.json"

    CASE_PROVIDER="govard"
    CASE_SESSION_ID="session-contract"
    CASE_RUN_ID="run-contract"
    CASE_PROJECT_ID="project-contract"
    CASE_TARGET_ID="target-contract"
    CASE_TARGET_MODE="standalone"
    CASE_TARGET_PATH="/host/workspace/sample-standalone-module"
    CASE_TARGET_RELATIVE=""
    CASE_IMAGE_DIGEST="sha256:1111111111111111111111111111111111111111111111111111111111111111"
    CASE_TOOLCHAIN_DIGEST="sha256:2222222222222222222222222222222222222222222222222222222222222222"
    CASE_PHP_VERSIONS=""
    CASE_MATRIX_COMPLETE=""
    CASE_PHPCS_MODE="pass"
    CASE_PHPCOMPAT_MODE="pass"
    CASE_PHPSTAN_MODE="pass"
    CASE_COMPOSER_MODE="pass"
    CASE_SLEEP_SECONDS="0"
    CASE_REPORT_SCRIPT="$CONTEXT_DIR/bin/report.php"
    REPORT="$CASE_DIR/output/report.json"
}

runner_environment() {
    RUNNER_ENV=(
        "PATH=$CASE_DIR/bin:$PATH"
        "HOME=$CASE_DIR/home"
        "GOVARD_LINT_PROVIDER=$CASE_PROVIDER"
        "GOVARD_LINT_SESSION_ID=$CASE_SESSION_ID"
        "GOVARD_LINT_RUN_ID=$CASE_RUN_ID"
        "GOVARD_LINT_PROJECT_ID=$CASE_PROJECT_ID"
        "GOVARD_LINT_TARGET_ID=$CASE_TARGET_ID"
        "GOVARD_LINT_TARGET_MODE=$CASE_TARGET_MODE"
        "GOVARD_LINT_TARGET_PATH=$CASE_TARGET_PATH"
        "GOVARD_LINT_TARGET_RELATIVE=$CASE_TARGET_RELATIVE"
        "GOVARD_LINT_PHP_VERSIONS=$CASE_PHP_VERSIONS"
        "GOVARD_LINT_MATRIX_COMPLETE=$CASE_MATRIX_COMPLETE"
        "GOVARD_LINT_IMAGE_DIGEST=$CASE_IMAGE_DIGEST"
        "GOVARD_LINT_TOOLCHAIN_DIGEST=$CASE_TOOLCHAIN_DIGEST"
        "GOVARD_LINT_SOURCE_DIR=$CASE_DIR/source"
        "GOVARD_LINT_CACHE_DIR=$CASE_DIR/cache"
        "GOVARD_LINT_OUTPUT_DIR=$CASE_DIR/output"
        "GOVARD_LINT_TOOLCHAIN_DIR=$CASE_DIR/toolchains"
        "GOVARD_LINT_WORKSPACE_DIR=$CASE_DIR/workspace"
        "GOVARD_LINT_REPORT_SCRIPT=$CASE_REPORT_SCRIPT"
        "GOVARD_LINT_REPORT_PHP=$TEST_PHP"
        "GOVARD_LINT_COMPOSER=$CASE_DIR/bin/composer"
        "GOVARD_LINT_COMPOSER_AUTH=$CASE_DIR/auth/auth.json"
        "GOVARD_TEST_PHPCS_MODE=$CASE_PHPCS_MODE"
        "GOVARD_TEST_PHPCOMPAT_MODE=$CASE_PHPCOMPAT_MODE"
        "GOVARD_TEST_PHPSTAN_MODE=$CASE_PHPSTAN_MODE"
        "GOVARD_TEST_COMPOSER_MODE=$CASE_COMPOSER_MODE"
        "GOVARD_TEST_SLEEP_SECONDS=$CASE_SLEEP_SECONDS"
    )
}

invoke_runner() {
    runner_environment
    env "${RUNNER_ENV[@]}" "$RUNNER" "$@" \
        >"$CASE_DIR/stdout.log" 2>"$CASE_DIR/stderr.log"
    RUN_STATUS=$?
    return 0
}

invoke_runner_background() {
    runner_environment
    env "${RUNNER_ENV[@]}" "$RUNNER" "$@" \
        >"$CASE_DIR/stdout.log" 2>"$CASE_DIR/stderr.log" &
    RUNNER_PID=$!
}

assert_valid_identity() {
    assert_equals "schema_version" "2" "$(json "$REPORT" schema_version)"
    assert_equals "provider" "$CASE_PROVIDER" "$(json "$REPORT" provider)"
    assert_equals "session_id" "$CASE_SESSION_ID" "$(json "$REPORT" session_id)"
    assert_equals "run_id" "$CASE_RUN_ID" "$(json "$REPORT" run_id)"
    assert_equals "project_id" "$CASE_PROJECT_ID" "$(json "$REPORT" project_id)"
    assert_equals "target_id" "$CASE_TARGET_ID" "$(json "$REPORT" target_id)"
    assert_equals "target_mode" "$CASE_TARGET_MODE" "$(json "$REPORT" target_mode)"
    assert_equals "target_path" "$CASE_TARGET_PATH" "$(json "$REPORT" target_path)"
    assert_equals "image_digest" "$CASE_IMAGE_DIGEST" "$(json "$REPORT" image_digest)"
    assert_equals "toolchain_digest" "$CASE_TOOLCHAIN_DIGEST" "$(json "$REPORT" toolchain_digest)"
}

# ---------------------------------------------------------------------------
# Cases
# ---------------------------------------------------------------------------

case_media_guard_reports_php_in_media() {
    new_case media_guard_reports_php_in_media
    CASE_TARGET_MODE="project"
    mkdir -p "$CASE_DIR/source/pub/media/custom_options/quote/b/y" \
        "$CASE_DIR/source/pub/media/catalog/product"
    printf '<?php eval(base64_decode($_REQUEST["id"]));\n' \
        >"$CASE_DIR/source/pub/media/custom_options/quote/b/y/bypass.php"
    printf 'binary upload\n' >"$CASE_DIR/source/pub/media/catalog/product/placeholder.png"
    invoke_runner --php 8.4
    assert_equals "exit status" "1" "$RUN_STATUS"
    assert_equals "status" "failed" "$(json "$REPORT" status)"
    assert_equals "phases" "validate:passed,prepare:passed,phpcs:passed,media-guard:failed,compat:passed,phpstan:passed" \
        "$(json_phases "$REPORT" 0)"
    assert_equals "media finding tool" "M2-LINT-MEDIA|PHP file in pub/media" \
        "$(json_finding_tools "$REPORT")"
    assert_equals "media finding path is target relative" "pub/media/custom_options/quote/b/y/bypass.php" \
        "$(json "$REPORT" php_results.0.findings.0.path)"
}

case_media_guard_passes_clean_media() {
    new_case media_guard_passes_clean_media
    CASE_TARGET_MODE="project"
    mkdir -p "$CASE_DIR/source/pub/media/catalog/product" "$CASE_DIR/source/app"
    printf 'binary upload\n' >"$CASE_DIR/source/pub/media/catalog/product/placeholder.png"
    printf '<?php\n' >"$CASE_DIR/source/app/code.php"
    invoke_runner --php 8.4
    assert_equals "exit status" "0" "$RUN_STATUS"
    assert_equals "status" "passed" "$(json "$REPORT" status)"
    assert_contains "phases" "$(json_phases "$REPORT" 0)" "media-guard:passed"
    assert_equals "no findings" "" "$(json_finding_tools "$REPORT")"
}

case_help_documents_contract() {
    new_case help_documents_contract
    invoke_runner --help
    local help_text
    help_text=$(cat "$CASE_DIR/stdout.log")
    assert_equals "exit status" "0" "$RUN_STATUS"
    assert_contains "help lists supported PHP versions" "$help_text" "7.4"
    assert_contains "help lists supported PHP versions" "$help_text" "8.5"
    assert_contains "help documents identity variables" "$help_text" "GOVARD_LINT_SESSION_ID"
    assert_contains "help documents the target mode flag" "$help_text" "--target-mode"
    assert_contains "help documents exit codes" "$help_text" "Exit codes"
    assert_file_absent "help writes no report" "$REPORT"
}

case_rejects_unknown_flag() {
    new_case rejects_unknown_flag
    invoke_runner --not-a-flag
    assert_equals "exit status" "64" "$RUN_STATUS"
    assert_contains "usage error is explained" "$(cat "$CASE_DIR/stderr.log")" "--not-a-flag"
    assert_file_absent "no report on usage error" "$REPORT"
}

case_rejects_unknown_linter() {
    new_case rejects_unknown_linter
    invoke_runner --linter psalm
    assert_equals "exit status" "64" "$RUN_STATUS"
    assert_contains "unknown linter is named" "$(cat "$CASE_DIR/stderr.log")" "psalm"
    assert_file_absent "no report on usage error" "$REPORT"
}

case_rejects_missing_identity() {
    new_case rejects_missing_identity
    CASE_SESSION_ID=""
    invoke_runner --php 8.2
    assert_equals "exit status" "64" "$RUN_STATUS"
    assert_contains "missing identity is named" "$(cat "$CASE_DIR/stderr.log")" "GOVARD_LINT_SESSION_ID"
    assert_file_absent "no report on usage error" "$REPORT"
}

case_project_php74_is_accepted() {
    new_case project_php74_is_accepted
    CASE_TARGET_MODE="project"
    CASE_TARGET_PATH="/host/workspace/shop"
    invoke_runner --php 7.4
    assert_equals "exit status" "0" "$RUN_STATUS"
    assert_file_exists "report written" "$REPORT"
    assert_valid_identity
    assert_equals "status" "passed" "$(json "$REPORT" status)"
    assert_equals "selected versions" '["7.4"]' "$(json "$REPORT" selected_php_versions)"
    assert_equals "matrix complete" "true" "$(json "$REPORT" matrix_complete)"
    assert_equals "result count" "1" "$(json_count "$REPORT" php_results)"
    assert_equals "result version" "7.4" "$(json_php_field "$REPORT" php_version)"
    assert_equals "result outcome" "passed" "$(json_php_field "$REPORT" outcome)"
    assert_equals "no findings" "" "$(json_finding_tools "$REPORT")"
    local phases
    phases=$(json_count "$REPORT" php_results.0.phases)
    if [ "$phases" = "0" ] || [ "$phases" = "<missing>" ]; then
        fail "php result carries no phase evidence"
    fi
}

case_project_php80_is_accepted() {
    new_case project_php80_is_accepted
    CASE_TARGET_MODE="project"
    CASE_TARGET_PATH="/host/workspace/shop"
    invoke_runner --php 8.0
    assert_equals "exit status" "0" "$RUN_STATUS"
    assert_file_exists "report written" "$REPORT"
    assert_valid_identity
    assert_equals "status" "passed" "$(json "$REPORT" status)"
    assert_equals "selected versions" '["8.0"]' "$(json "$REPORT" selected_php_versions)"
    assert_equals "matrix complete" "true" "$(json "$REPORT" matrix_complete)"
    assert_equals "result count" "1" "$(json_count "$REPORT" php_results)"
    assert_equals "result version" "8.0" "$(json_php_field "$REPORT" php_version)"
    assert_equals "result outcome" "passed" "$(json_php_field "$REPORT" outcome)"
    assert_equals "no findings" "" "$(json_finding_tools "$REPORT")"
    # Like 7.4, the 8.0 toolchain resolves magento/magento-coding-standard 4,
    # which ships no PHP compatibility standard, so that phase is skipped.
    assert_equals "phases" "validate:passed,prepare:passed,phpcs:passed,media-guard:passed,compat:skipped,phpstan:passed" \
        "$(json_phases "$REPORT" 0)"
}

case_project_rejects_multiple_php() {
    new_case project_rejects_multiple_php
    CASE_TARGET_MODE="project"
    invoke_runner --php 7.4,8.2
    assert_equals "exit status" "64" "$RUN_STATUS"
    assert_contains "single active PHP is required" "$(cat "$CASE_DIR/stderr.log")" "single"
    assert_file_absent "no report on usage error" "$REPORT"
}

case_unsupported_php_is_reported() {
    new_case unsupported_php_is_reported
    CASE_TARGET_MODE="project"
    # 7.3 is below the supported floor in every target mode.
    invoke_runner --php 7.3
    assert_equals "exit status" "3" "$RUN_STATUS"
    assert_file_exists "report written" "$REPORT"
    assert_equals "status" "unsupported" "$(json "$REPORT" status)"
    assert_equals "result outcome" "unsupported" "$(json_php_field "$REPORT" outcome)"
    local phases
    phases=$(json_count "$REPORT" php_results.0.phases)
    if [ "$phases" = "0" ] || [ "$phases" = "<missing>" ]; then
        fail "unsupported result carries no phase evidence"
    fi
}

# PHP 8.0 is a project and module-in-project option only. The standalone
# default matrix stays 8.1-8.5, so a standalone module must still refuse it.
case_standalone_php80_is_unsupported() {
    new_case standalone_php80_is_unsupported
    invoke_runner --php 8.0
    assert_equals "exit status" "3" "$RUN_STATUS"
    assert_file_exists "report written" "$REPORT"
    assert_equals "target mode" "standalone" "$(json "$REPORT" target_mode)"
    assert_equals "status" "unsupported" "$(json "$REPORT" status)"
    assert_equals "selected versions" '["8.0"]' "$(json "$REPORT" selected_php_versions)"
    assert_equals "result outcome" "unsupported" "$(json_php_field "$REPORT" outcome)"
    assert_equals "matrix complete" "false" "$(json "$REPORT" matrix_complete)"
    local phases
    phases=$(json_count "$REPORT" php_results.0.phases)
    if [ "$phases" = "0" ] || [ "$phases" = "<missing>" ]; then
        fail "unsupported result carries no phase evidence"
    fi
}

case_unsupported_php_mixes_with_supported() {
    new_case unsupported_php_mixes_with_supported
    invoke_runner --php 7.3,8.1
    assert_equals "exit status" "0" "$RUN_STATUS"
    assert_equals "status" "passed" "$(json "$REPORT" status)"
    assert_equals "selected versions" '["7.3","8.1"]' "$(json "$REPORT" selected_php_versions)"
    assert_equals "outcomes" "unsupported,passed" "$(json_php_field "$REPORT" outcome)"
    assert_equals "matrix complete" "false" "$(json "$REPORT" matrix_complete)"
}

case_standalone_runs_default_matrix() {
    new_case standalone_runs_default_matrix
    invoke_runner
    assert_equals "exit status" "0" "$RUN_STATUS"
    assert_equals "status" "passed" "$(json "$REPORT" status)"
    assert_equals "result count" "5" "$(json_count "$REPORT" php_results)"
    assert_equals "result versions" "8.1,8.2,8.3,8.4,8.5" "$(json_php_field "$REPORT" php_version)"
    assert_equals "matrix complete" "true" "$(json "$REPORT" matrix_complete)"
    assert_equals "selected versions" '["8.1","8.2","8.3","8.4","8.5"]' "$(json "$REPORT" selected_php_versions)"
}

case_standalone_narrows_matrix() {
    new_case standalone_narrows_matrix
    invoke_runner --php 8.1 --php 8.5
    assert_equals "exit status" "0" "$RUN_STATUS"
    assert_equals "result count" "2" "$(json_count "$REPORT" php_results)"
    assert_equals "result versions" "8.1,8.5" "$(json_php_field "$REPORT" php_version)"
    assert_equals "matrix complete" "false" "$(json "$REPORT" matrix_complete)"
}

case_findings_are_normalized() {
    new_case findings_are_normalized
    CASE_PHPCS_MODE="findings"
    CASE_PHPSTAN_MODE="findings"
    invoke_runner --php 8.3
    assert_equals "exit status" "1" "$RUN_STATUS"
    assert_equals "status" "failed" "$(json "$REPORT" status)"
    assert_equals "result outcome" "failed" "$(json_php_field "$REPORT" outcome)"
    assert_equals "finding tools" \
        "M2-LINT-PHPCS|Generic.Files.LineLength.TooLong,M2-LINT-PHPSTAN|missingType.parameter" \
        "$(json_finding_tools "$REPORT")"
    assert_equals "finding path is target relative" "Model/Greeting.php" \
        "$(json "$REPORT" php_results.0.findings.0.path)"
    assert_equals "finding line" "12" "$(json "$REPORT" php_results.0.findings.0.line)"
}

case_compatibility_findings_are_normalized() {
    new_case compatibility_findings_are_normalized
    CASE_PHPCS_MODE="internal"
    CASE_PHPSTAN_MODE="compat"
    invoke_runner --php 8.1 --php 8.2
    assert_equals "exit status" "1" "$RUN_STATUS"
    assert_equals "status" "failed" "$(json "$REPORT" status)"
    assert_equals "internal sniff failures and phpstan global errors are compatibility findings" \
        "M2-LINT-COMPAT|Internal.Exception,M2-LINT-COMPAT|,M2-LINT-COMPAT|Internal.Exception,M2-LINT-COMPAT|" \
        "$(json_finding_tools "$REPORT")"
    assert_equals "both versions still ran" "8.1,8.2" "$(json_php_field "$REPORT" php_version)"
    assert_equals "both versions failed" "failed,failed" "$(json_php_field "$REPORT" outcome)"
}

case_compatibility_pass_reports_version_findings() {
    new_case compatibility_pass_reports_version_findings
    CASE_PHPCOMPAT_MODE="findings"
    invoke_runner --php 8.2
    assert_equals "exit status" "1" "$RUN_STATUS"
    assert_equals "status" "failed" "$(json "$REPORT" status)"
    assert_equals "phases" "validate:passed,prepare:passed,phpcs:passed,media-guard:passed,compat:failed,phpstan:passed" \
        "$(json_phases "$REPORT" 0)"
    assert_equals "compatibility findings" \
        "M2-LINT-COMPAT|PHPCompatibility.FunctionUse.RemovedFunctions.create_functionDeprecatedRemoved" \
        "$(json_finding_tools "$REPORT")"
    assert_contains "the analyzed version is passed to the compatibility pass" \
        "$(json "$REPORT" php_results.0.findings.0.message)" "PHP 8.2"
}

case_compatibility_pass_skips_without_standard() {
    new_case compatibility_pass_skips_without_standard
    CASE_TARGET_MODE="project"
    CASE_PHPCOMPAT_MODE="findings"
    invoke_runner --php 7.4
    assert_equals "exit status" "0" "$RUN_STATUS"
    assert_equals "status" "passed" "$(json "$REPORT" status)"
    assert_equals "phases" "validate:passed,prepare:passed,phpcs:passed,media-guard:passed,compat:skipped,phpstan:passed" \
        "$(json_phases "$REPORT" 0)"
    assert_equals "no findings" "" "$(json_finding_tools "$REPORT")"
}

case_infrastructure_failure_publishes_report() {
    new_case infrastructure_failure_publishes_report
    CASE_REPORT_SCRIPT="$CASE_DIR/missing-report.php"
    invoke_runner --php 8.1 --php 8.2
    assert_equals "exit status" "2" "$RUN_STATUS"
    assert_file_exists "an infrastructure report is still published" "$REPORT"
    assert_valid_identity
    assert_equals "status" "infra_error" "$(json "$REPORT" status)"
    assert_equals "selected versions" '["8.1","8.2"]' "$(json "$REPORT" selected_php_versions)"
    assert_equals "outcomes" "infra_error,infra_error" "$(json_php_field "$REPORT" outcome)"
    assert_equals "phase evidence" "prepare:error" "$(json_phases "$REPORT" 0)"
    local partials
    partials=$(find "$CASE_DIR/output" -name '*.tmp*' 2>/dev/null | wc -l | tr -d ' ')
    assert_equals "no temporary file remains" "0" "$partials"
}

case_infrastructure_failure_is_reported() {
    new_case infrastructure_failure_is_reported
    CASE_PHPCS_MODE="error"
    invoke_runner --php 8.2 --php 8.3
    assert_equals "exit status" "2" "$RUN_STATUS"
    assert_equals "status" "infra_error" "$(json "$REPORT" status)"
    assert_equals "outcomes" "infra_error,infra_error" "$(json_php_field "$REPORT" outcome)"
}

case_cache_state_transitions() {
    new_case cache_state_transitions
    invoke_runner --php 8.3
    assert_equals "cold exit status" "0" "$RUN_STATUS"
    assert_equals "cold cache state" "cold" "$(json_php_field "$REPORT" cache.state)"
    invoke_runner --php 8.3
    assert_equals "warm exit status" "0" "$RUN_STATUS"
    assert_equals "warm cache state" "warm" "$(json_php_field "$REPORT" cache.state)"
    invoke_runner --php 8.3 --no-result-cache
    assert_equals "bypassed exit status" "0" "$RUN_STATUS"
    assert_equals "bypassed cache state" "bypassed" "$(json_php_field "$REPORT" cache.state)"
    local key
    key=$(json "$REPORT" php_results.0.cache.key)
    if [ -z "$key" ] || [ "$key" = "<missing>" ]; then
        fail "cache evidence carries no key"
    fi
}

case_source_tree_is_unchanged() {
    new_case source_tree_is_unchanged
    local before after
    before=$(tree_digest "$CASE_DIR/source")
    chmod -R a-w "$CASE_DIR/source"
    invoke_runner --php 8.1 --php 8.2
    chmod -R u+w "$CASE_DIR/source"
    after=$(tree_digest "$CASE_DIR/source")
    assert_equals "exit status" "0" "$RUN_STATUS"
    assert_equals "source digest" "$before" "$after"
    assert_file_absent "no vendor tree in source" "$CASE_DIR/source/vendor"
}

case_report_is_written_atomically() {
    new_case report_is_written_atomically
    CASE_PHPCS_MODE="sleep"
    CASE_SLEEP_SECONDS="3"
    invoke_runner_background --php 8.4
    sleep 1
    assert_file_absent "no partial report while analyzing" "$REPORT"
    local partials
    partials=$(find "$CASE_DIR/output" -name '*.tmp*' 2>/dev/null | wc -l | tr -d ' ')
    assert_equals "no temporary report left visible" "0" "$partials"
    wait "$RUNNER_PID"
    RUN_STATUS=$?
    assert_equals "exit status" "0" "$RUN_STATUS"
    assert_file_exists "report written" "$REPORT"
    partials=$(find "$CASE_DIR/output" -name '*.tmp*' 2>/dev/null | wc -l | tr -d ' ')
    assert_equals "no temporary file remains" "0" "$partials"
}

case_sigterm_cancels_run() {
    new_case sigterm_cancels_run
    CASE_PHPCS_MODE="sleep"
    CASE_SLEEP_SECONDS="20"
    invoke_runner_background --php 8.1 --php 8.2 --php 8.3
    sleep 1
    kill -TERM "$RUNNER_PID" 2>/dev/null
    wait "$RUNNER_PID"
    RUN_STATUS=$?
    assert_equals "exit status" "143" "$RUN_STATUS"
    assert_file_exists "cancellation report written" "$REPORT"
    assert_equals "status" "cancelled" "$(json "$REPORT" status)"
    assert_equals "all versions reported" "8.1,8.2,8.3" "$(json_php_field "$REPORT" php_version)"
    assert_equals "outcomes" "cancelled,cancelled,cancelled" "$(json_php_field "$REPORT" outcome)"
}

case_auth_values_never_surface() {
    new_case auth_values_never_surface
    invoke_runner --php 8.1
    assert_equals "exit status" "0" "$RUN_STATUS"
    assert_not_contains "stdout hides credentials" "$(cat "$CASE_DIR/stdout.log")" "$AUTH_TOKEN"
    assert_not_contains "stderr hides credentials" "$(cat "$CASE_DIR/stderr.log")" "$AUTH_TOKEN"
    assert_not_contains "report hides credentials" "$(cat "$REPORT")" "$AUTH_TOKEN"
    local leaks
    leaks=$(grep -RIl -F "$AUTH_TOKEN" "$CASE_DIR/cache" "$CASE_DIR/output" 2>/dev/null | wc -l | tr -d ' ')
    assert_equals "cache and output hide credentials" "0" "$leaks"
}

case_phpstan_stale_cache_is_healed() {
    new_case phpstan_stale_cache_is_healed
    CASE_PHPSTAN_MODE="stale"
    invoke_runner --php 8.3
    # Glint should detect the hash_file stale cache error, purge phpstan tmpDir and retry once.
    assert_equals "stale cache healed exit status" "0" "$RUN_STATUS"
    assert_equals "stale cache healed outcome" "passed" "$(json_php_field "$REPORT" outcome)"
    assert_contains "log mentions purge" "$(cat "$CASE_DIR/stdout.log" "$CASE_DIR/stderr.log")" "stale resultCache"
    # Verify the phpstan cache was actually purged (marker removed by retry's mkdir, stale marker remains but phpstan dir was cleared)
    local cache_key
    cache_key=$(json "$REPORT" php_results.0.cache.key)
    if [ -z "$cache_key" ] || [ "$cache_key" = "<missing>" ]; then
        fail "stale cache: cache evidence carries no key after heal"
    fi
    # Second warm run without stale mode should stay warm/cold without needing another heal.
    CASE_PHPSTAN_MODE="pass"
    invoke_runner --php 8.3
    assert_equals "warm after heal exit status" "0" "$RUN_STATUS"
    assert_equals "warm after heal outcome" "passed" "$(json_php_field "$REPORT" outcome)"
}

case_phpstan_excludes_var_deployer() {
    new_case phpstan_excludes_var_deployer
    CASE_TARGET_MODE="project"
    # Create a var/deployer tree that would trigger the original hash_file bug if not excluded.
    mkdir -p "$CASE_DIR/source/var/deployer/releases/1/var/www/html/app/etc"
    printf '<?php return ["backend"=>["frontName"=>"admin"]];\n' >"$CASE_DIR/source/var/deployer/releases/1/var/www/html/app/etc/env.php"
    # Verify write_phpstan_config emits a recursive exclude for var.
    invoke_runner --php 8.3
    assert_equals "exclude var exit status" "0" "$RUN_STATUS"
    local config_file
    config_file=$(find "$CASE_DIR/workspace" -name "phpstan.neon" 2>/dev/null | head -1)
    if [ -z "$config_file" ]; then
        fail "phpstan config not found"
    else
        assert_contains "phpstan config excludes var recursively" "$(cat "$config_file")" "var/**/*"
        assert_not_contains "phpstan config still uses shallow var/*" "$(cat "$config_file")" 'var/*"'
        # The var file should not contribute to findings (it is excluded) - report should be clean.
        assert_equals "var file not analyzed" "passed" "$(json_php_field "$REPORT" outcome)"
    fi
}

case_phpstan_excludes_test_fixtures() {
    new_case phpstan_excludes_test_fixtures
    CASE_TARGET_MODE="project"
    # Framework test fixtures (dev/tests) crash PHPStan with severe internal
    # errors on full-project analysis, discarding every per-file finding.
    mkdir -p "$CASE_DIR/source/dev/tests/integration/_files"
    printf '<?php class Fixture_Broken {}\n' >"$CASE_DIR/source/dev/tests/integration/_files/broken.php"
    # Verify write_phpstan_config emits a recursive exclude for dev/tests.
    invoke_runner --php 8.3
    assert_equals "exclude fixtures exit status" "0" "$RUN_STATUS"
    local config_file
    config_file=$(find "$CASE_DIR/workspace" -name "phpstan.neon" 2>/dev/null | head -1)
    if [ -z "$config_file" ]; then
        fail "phpstan config not found"
    else
        assert_contains "phpstan config excludes dev/tests recursively" "$(cat "$config_file")" "dev/tests/**/*"
        # The fixture file should not contribute to findings (it is excluded) - report should be clean.
        assert_equals "fixture file not analyzed" "passed" "$(json_php_field "$REPORT" outcome)"
    fi
}

CASES="
case_help_documents_contract
case_rejects_unknown_flag
case_rejects_unknown_linter
case_rejects_missing_identity
case_project_php74_is_accepted
case_project_php80_is_accepted
case_project_rejects_multiple_php
case_unsupported_php_is_reported
case_standalone_php80_is_unsupported
case_unsupported_php_mixes_with_supported
case_standalone_runs_default_matrix
case_standalone_narrows_matrix
case_findings_are_normalized
case_compatibility_findings_are_normalized
case_compatibility_pass_reports_version_findings
case_compatibility_pass_skips_without_standard
case_infrastructure_failure_is_reported
case_infrastructure_failure_publishes_report
case_cache_state_transitions
case_source_tree_is_unchanged
case_report_is_written_atomically
case_sigterm_cancels_run
case_auth_values_never_surface
case_media_guard_reports_php_in_media
case_media_guard_passes_clean_media
case_phpstan_stale_cache_is_healed
case_phpstan_excludes_var_deployer
case_phpstan_excludes_test_fixtures
"

run_case() {
    local function_name=$1
    cases_run=$((cases_run + 1))
    "$function_name"
    if [ "$case_failures" -eq 0 ]; then
        printf 'ok %d - %s\n' "$cases_run" "$case_name"
    else
        cases_failed=$((cases_failed + 1))
        printf 'not ok %d - %s\n' "$cases_run" "$case_name"
        if [ -s "$CASE_DIR/stderr.log" ]; then
            while IFS= read -r line; do
                printf '  # stderr: %s\n' "$line"
            done <"$CASE_DIR/stderr.log"
        fi
    fi
}

main() {
    if ! TEST_PHP=$(detect_php); then
        if [ "${GOVARD_GLINT_TEST_INNER:-0}" = "1" ]; then
            printf 'contract tests need a PHP CLI inside the container\n' >&2
            return 1
        fi
        reexec_in_container "$@"
        return $?
    fi
    export TEST_PHP

    if [ ! -f "$RUNNER" ]; then
        printf 'contract tests require the runner at %s\n' "$RUNNER" >&2
        return 1
    fi
    if [ ! -x "$RUNNER" ]; then
        printf 'contract tests require %s to be executable\n' "$RUNNER" >&2
        return 1
    fi

    WORK_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/glint-contract.XXXXXX") || return 1
    trap 'chmod -R u+w "$WORK_ROOT" 2>/dev/null; rm -rf "$WORK_ROOT"' EXIT

    local selected="$CASES"
    if [ "$#" -gt 0 ]; then
        selected=""
        local requested
        for requested in "$@"; do
            case "$requested" in
                case_*) selected="$selected $requested" ;;
                *) selected="$selected case_$requested" ;;
            esac
        done
    fi

    local function_name
    for function_name in $selected; do
        run_case "$function_name"
    done

    printf '# %d cases, %d failed\n' "$cases_run" "$cases_failed"
    if [ "$cases_failed" -ne 0 ]; then
        return 1
    fi
    return 0
}

main "$@"
