<?php
/**
 * Govard Magento lint reporter.
 *
 * Reads the record stream written by magelint, normalizes PHPCS and
 * PHPStan output into Govard lint findings, and writes one schema-v2 lint
 * report atomically. The aggregate status is printed on stdout so the runner
 * can map it to a process exit code.
 *
 * Usage:
 *   php report.php --records <records-file> --output <report-path>
 *
 * The report identity is read from the GOVARD_LINT_* environment contract; no
 * credential material is read, logged, or embedded.
 *
 * Copyright (c) Govard contributors.
 * Distributed under the terms of the repository LICENSE file.
 */

declare(strict_types=1);

const REPORT_SCHEMA_VERSION = 2;
const TOOL_PHPCS = 'M2-LINT-PHPCS';
const TOOL_PHPSTAN = 'M2-LINT-PHPSTAN';
const TOOL_COMPAT = 'M2-LINT-COMPAT';
const MESSAGE_LIMIT = 2000;

/**
 * Parse the reporter command line.
 *
 * @param string[] $argv
 * @return array{records: string, output: string}
 */
function parse_options(array $argv): array
{
    $options = ['records' => '', 'output' => ''];
    $arguments = array_slice($argv, 1);
    for ($index = 0; $index < count($arguments); $index++) {
        $argument = $arguments[$index];
        $name = $argument;
        $value = null;
        $separator = strpos($argument, '=');
        if ($separator !== false) {
            $name = substr($argument, 0, $separator);
            $value = substr($argument, $separator + 1);
        }
        if ($name !== '--records' && $name !== '--output') {
            fail('unknown reporter option ' . $argument);
        }
        if ($value === null) {
            $index++;
            if ($index >= count($arguments)) {
                fail('reporter option ' . $name . ' needs a value');
            }
            $value = $arguments[$index];
        }
        $options[substr($name, 2)] = $value;
    }
    if ($options['records'] === '' || $options['output'] === '') {
        fail('reporter needs --records and --output');
    }
    return $options;
}

function fail(string $message): void
{
    fwrite(STDERR, 'magelint report: ' . $message . PHP_EOL);
    exit(1);
}

/**
 * Decode the base64 record stream written by the runner.
 *
 * @return array<int, array{type: string, fields: string[]}>
 */
function read_records(string $path): array
{
    $handle = @fopen($path, 'rb');
    if ($handle === false) {
        fail('cannot read records ' . $path);
    }
    $records = [];
    while (($line = fgets($handle)) !== false) {
        $line = trim($line);
        if ($line === '') {
            continue;
        }
        $parts = explode(' ', $line);
        $type = array_shift($parts);
        $fields = [];
        foreach ($parts as $part) {
            $decoded = base64_decode($part, true);
            $fields[] = $decoded === false ? '' : $decoded;
        }
        $records[] = ['type' => $type, 'fields' => $fields];
    }
    fclose($handle);
    return $records;
}

function field(array $fields, int $index): string
{
    return array_key_exists($index, $fields) ? $fields[$index] : '';
}

function normalize_message(string $message): string
{
    $message = str_replace(["\r\n", "\r", "\n", "\t"], ' ', $message);
    $message = trim(preg_replace('/ {2,}/', ' ', $message) ?? $message);
    if (strlen($message) > MESSAGE_LIMIT) {
        $message = substr($message, 0, MESSAGE_LIMIT - 3) . '...';
    }
    return $message;
}

function relative_path(string $path, string $basePath): string
{
    if ($path === '') {
        return '';
    }
    if ($basePath !== '') {
        $prefix = rtrim($basePath, '/') . '/';
        if (strpos($path, $prefix) === 0) {
            $path = substr($path, strlen($prefix));
        }
    }
    if (strpos($path, './') === 0) {
        $path = substr($path, 2);
    }
    return $path;
}

/**
 * @return array<int, array<string, mixed>>
 */
function finding(string $tool, string $rule, string $path, int $line, int $column, string $message): array
{
    return [
        'tool' => $tool,
        'rule' => $rule,
        'path' => $path,
        'line' => $line,
        'column' => $column,
        'message' => normalize_message($message),
    ];
}

/**
 * Normalize a PHPCS JSON report. Internal sniff failures describe code the
 * analyzer could not process on this PHP version, so they are compatibility
 * findings rather than style findings. A pass run against the PHP
 * compatibility standard reports every message as a compatibility finding.
 *
 * @param string|null $forcedTool
 * @return array<int, array<string, mixed>>
 */
function normalize_phpcs(array $payload, string $basePath, $forcedTool = null): array
{
    $findings = [];
    $files = isset($payload['files']) && is_array($payload['files']) ? $payload['files'] : [];
    foreach ($files as $file => $data) {
        $messages = isset($data['messages']) && is_array($data['messages']) ? $data['messages'] : [];
        foreach ($messages as $message) {
            $rule = (string) ($message['source'] ?? '');
            $tool = TOOL_PHPCS;
            if (strpos($rule, 'Internal.') === 0 || strpos($rule, 'PHPCompatibility') === 0) {
                // Internal sniff failures and compatibility sniffs both describe
                // code the analyzed PHP version cannot carry, whichever pass
                // reported them.
                $tool = TOOL_COMPAT;
            }
            if ($forcedTool !== null) {
                $tool = $forcedTool;
            }
            $findings[] = finding(
                $tool,
                $rule,
                relative_path((string) $file, $basePath),
                (int) ($message['line'] ?? 0),
                (int) ($message['column'] ?? 0),
                (string) ($message['message'] ?? '')
            );
        }
    }
    return $findings;
}

/**
 * Normalize a PHPStan JSON report. Non-file errors are reported against the
 * analyzed PHP version itself and are normalized as compatibility findings.
 *
 * @return array<int, array<string, mixed>>
 */
function normalize_phpstan(array $payload, string $basePath): array
{
    $findings = [];
    $files = isset($payload['files']) && is_array($payload['files']) ? $payload['files'] : [];
    foreach ($files as $file => $data) {
        $messages = isset($data['messages']) && is_array($data['messages']) ? $data['messages'] : [];
        foreach ($messages as $message) {
            $findings[] = finding(
                TOOL_PHPSTAN,
                (string) ($message['identifier'] ?? ''),
                relative_path((string) $file, $basePath),
                (int) ($message['line'] ?? 0),
                0,
                (string) ($message['message'] ?? '')
            );
        }
    }
    $errors = isset($payload['errors']) && is_array($payload['errors']) ? $payload['errors'] : [];
    foreach ($errors as $error) {
        $findings[] = finding(TOOL_COMPAT, '', '', 0, 0, is_string($error) ? $error : json_encode($error));
    }
    return $findings;
}

/**
 * @return array{status: string, findings: array<int, array<string, mixed>>, phases: array<int, array<string, mixed>>}
 */
function normalize_artifact(string $tool, string $format, string $path, string $basePath): array
{
    $label = $format === 'phpstan-json' ? 'phpstan' : ($format === 'phpcs-compat-json' ? 'compat' : 'phpcs');
    $raw = is_file($path) ? @file_get_contents($path) : false;
    if ($raw === false || trim((string) $raw) === '') {
        return [
            'status' => 'error',
            'findings' => [],
            'phases' => [[
                'name' => $label . '-report',
                'status' => 'error',
                'duration_ms' => 0,
                'detail' => 'missing analyzer report',
            ]],
        ];
    }
    $payload = json_decode((string) $raw, true);
    if (!is_array($payload)) {
        return [
            'status' => 'error',
            'findings' => [],
            'phases' => [[
                'name' => $label . '-report',
                'status' => 'error',
                'duration_ms' => 0,
                'detail' => 'unreadable analyzer report',
            ]],
        ];
    }
    if ($format === 'phpstan-json') {
        $findings = normalize_phpstan($payload, $basePath);
    } elseif ($format === 'phpcs-compat-json') {
        $findings = normalize_phpcs($payload, $basePath, TOOL_COMPAT);
    } else {
        $findings = normalize_phpcs($payload, $basePath);
    }
    return ['status' => 'ok', 'findings' => $findings, 'phases' => []];
}

/**
 * Drop findings repeated by two passes over the same code. The Magento coding
 * standard bundles compatibility sniffs that the dedicated compatibility pass
 * reports again; identical tool, rule, location, and message are one finding.
 *
 * @param array<int, array<string, mixed>> $findings
 * @return array<int, array<string, mixed>>
 */
function deduplicate_findings(array $findings): array
{
    $unique = [];
    $seen = [];
    foreach ($findings as $candidate) {
        $key = implode("\0", [
            (string) $candidate['tool'],
            (string) $candidate['rule'],
            (string) $candidate['path'],
            (string) $candidate['line'],
            (string) $candidate['column'],
            (string) $candidate['message'],
        ]);
        if (isset($seen[$key])) {
            continue;
        }
        $seen[$key] = true;
        $unique[] = $candidate;
    }
    return $unique;
}

/**
 * Aggregate per-PHP outcomes exactly like the Govard lint report validator.
 *
 * @param string[] $outcomes
 */
function aggregate_status(array $outcomes): string
{
    if ($outcomes === []) {
        return 'infra_error';
    }
    if (in_array('cancelled', $outcomes, true)) {
        return 'cancelled';
    }
    if (in_array('infra_error', $outcomes, true)) {
        return 'infra_error';
    }
    if (in_array('failed', $outcomes, true)) {
        return 'failed';
    }
    if (in_array('passed', $outcomes, true)) {
        return 'passed';
    }
    return 'unsupported';
}

function write_atomically(string $path, string $content): void
{
    $directory = dirname($path);
    if (!is_dir($directory) && !@mkdir($directory, 0755, true) && !is_dir($directory)) {
        fail('cannot create report directory ' . $directory);
    }
    $temporary = $path . '.tmp' . getmypid();
    if (@file_put_contents($temporary, $content) === false) {
        fail('cannot write report ' . $temporary);
    }
    @chmod($temporary, 0600);
    if (!@rename($temporary, $path)) {
        @unlink($temporary);
        fail('cannot publish report ' . $path);
    }
}

$options = parse_options($argv);
$records = read_records($options['records']);

$selected = [];
$matrixComplete = false;
$totalDuration = 0;
$phases = [];
$outcomeMarkers = [];
$durations = [];
$caches = [];
$findings = [];
$artifacts = [];

foreach ($records as $record) {
    $fields = $record['fields'];
    switch ($record['type']) {
        case 'selected':
            $value = field($fields, 0);
            $selected = $value === '' ? [] : explode(',', $value);
            break;
        case 'matrix':
            $matrixComplete = field($fields, 0) === 'true';
            break;
        case 'total':
            $totalDuration = (int) field($fields, 0);
            break;
        case 'phase':
            $version = field($fields, 0);
            $phases[$version][] = [
                'name' => field($fields, 1),
                'php_version' => $version,
                'status' => field($fields, 2),
                'duration_ms' => (int) field($fields, 3),
                'cache_state' => field($fields, 4),
                'cache_key' => field($fields, 5),
                'cache_reason' => field($fields, 6),
            ];
            break;
        case 'php':
            $version = field($fields, 0);
            $outcomeMarkers[$version] = field($fields, 1);
            $durations[$version] = (int) field($fields, 2);
            $caches[$version] = [
                'state' => field($fields, 3),
                'key' => field($fields, 4),
                'reason' => field($fields, 5),
            ];
            break;
        case 'finding':
            $version = field($fields, 0);
            $findings[$version][] = finding(
                field($fields, 1),
                field($fields, 2),
                field($fields, 3),
                (int) field($fields, 4),
                (int) field($fields, 5),
                field($fields, 6)
            );
            break;
        case 'artifact':
            $version = field($fields, 0);
            $artifacts[$version][] = [
                'tool' => field($fields, 1),
                'format' => field($fields, 2),
                'path' => field($fields, 3),
                'basepath' => field($fields, 4),
            ];
            break;
        default:
            break;
    }
}

$results = [];
$outcomes = [];
foreach ($selected as $version) {
    $versionPhases = $phases[$version] ?? [];
    $versionFindings = $findings[$version] ?? [];
    $marker = $outcomeMarkers[$version] ?? 'infra_error';
    $artifactFailed = false;
    foreach ($artifacts[$version] ?? [] as $artifact) {
        $normalized = normalize_artifact(
            $artifact['tool'],
            $artifact['format'],
            $artifact['path'],
            $artifact['basepath']
        );
        if ($normalized['status'] !== 'ok') {
            $artifactFailed = true;
        }
        foreach ($normalized['phases'] as $extra) {
            $versionPhases[] = [
                'name' => $extra['name'],
                'php_version' => $version,
                'status' => $extra['status'],
                'duration_ms' => 0,
                'cache_state' => '',
                'cache_key' => '',
                'cache_reason' => $extra['detail'],
            ];
        }
        foreach ($normalized['findings'] as $normalizedFinding) {
            $versionFindings[] = $normalizedFinding;
        }
    }
    foreach ($versionPhases as $phase) {
        if ($phase['status'] === 'error') {
            $artifactFailed = true;
        }
    }
    if ($versionPhases === []) {
        $versionPhases[] = [
            'name' => 'validate',
            'php_version' => $version,
            'status' => $marker === 'cancelled' ? 'cancelled' : 'error',
            'duration_ms' => 0,
            'cache_state' => '',
            'cache_key' => '',
            'cache_reason' => 'no phase evidence recorded',
        ];
        if ($marker !== 'cancelled') {
            $artifactFailed = true;
        }
    }
    if ($marker === 'cancelled') {
        $outcome = 'cancelled';
    } elseif ($marker === 'unsupported') {
        $outcome = 'unsupported';
    } elseif ($marker === 'infra_error' || $artifactFailed) {
        $outcome = 'infra_error';
    } else {
        $outcome = $versionFindings === [] ? 'passed' : 'failed';
    }
    $outcomes[] = $outcome;
    $results[] = [
        'php_version' => $version,
        'outcome' => $outcome,
        'duration_ms' => $durations[$version] ?? 0,
        'cache' => $caches[$version] ?? ['state' => 'cold', 'key' => '', 'reason' => ''],
        'phases' => array_values($versionPhases),
        'findings' => deduplicate_findings($versionFindings),
    ];
}

$status = aggregate_status($outcomes);
$report = [
    'schema_version' => REPORT_SCHEMA_VERSION,
    'provider' => (string) getenv('GOVARD_LINT_PROVIDER'),
    'session_id' => (string) getenv('GOVARD_LINT_SESSION_ID'),
    'run_id' => (string) getenv('GOVARD_LINT_RUN_ID'),
    'project_id' => (string) getenv('GOVARD_LINT_PROJECT_ID'),
    'target_id' => (string) getenv('GOVARD_LINT_TARGET_ID'),
    'target_mode' => (string) getenv('GOVARD_LINT_TARGET_MODE'),
    'target_path' => (string) getenv('GOVARD_LINT_TARGET_PATH'),
    'image_digest' => (string) getenv('GOVARD_LINT_IMAGE_DIGEST'),
    'toolchain_digest' => (string) getenv('GOVARD_LINT_TOOLCHAIN_DIGEST'),
    'status' => $status,
    'duration_ms' => $totalDuration,
    'selected_php_versions' => array_values($selected),
    'matrix_complete' => $matrixComplete,
    'php_results' => $results,
];

$encoded = json_encode($report, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES);
if ($encoded === false) {
    fail('cannot encode report');
}
write_atomically($options['output'], $encoded . PHP_EOL);
echo $status;
exit(0);
