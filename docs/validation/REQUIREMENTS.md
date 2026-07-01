# Hermes Functional Requirements

## Document Information

**Product**: Hermes - Cloud-Native HPC Command Proxy
**Version**: 0.1.0
**Purpose**: CFR 21 Part 11 Compliance - GAMP 5 Validation
**Status**: Active Development

## Overview

This document establishes functional requirements for Hermes command proxy system. Requirements are designed to support CFR 21 Part 11 compliance for use in GxP (Good Practice) environments within pharmaceutical/life sciences industries.

### Compliance Framework

**CFR 21 Part 11 Key Principles**:
- Electronic records must be trustworthy, reliable, and equivalent to paper records
- Systems must be validated to ensure accuracy, reliability, and performance
- Audit trails must capture who did what, when, and why
- Data integrity must be maintained (ALCOA+ principles)

**GAMP 5 Classification**: Category 4 (Configured Product)
- Custom Go application
- Requires prospective validation
- Risk-based testing approach

### Validation Strategy

Per CLAUDE.md:
- Requirements are assigned unique IDs (REQ-XXX-NNN)
- Tests map back to requirements via metadata
- Not all tests map to requirements (unit tests for implementation details)
- Validation tests prove/satisfy specific requirements

---

## Requirement Categories

- **EXE**: Command Execution
- **FILE**: File Management
- **AUD**: Audit Trail & Traceability
- **SEC**: Security & Access Control
- **VAL**: Validation & Verification
- **CFG**: Configuration Management
- **ERR**: Error Handling

---

## Local Mode Requirements

### EXE-LOC: Local Execution Mode

#### REQ-EXE-LOC-001: Local Command Execution
**Priority**: Critical
**GAMP Category**: GxP Critical

**Requirement**:
The system SHALL execute commands directly on the host filesystem when configured in local mode.

**Rationale**:
Provides lightweight execution environment for development and testing without Docker dependency.

**Acceptance Criteria**:
- System creates isolated workspace directory per execution
- Command executes with specified arguments
- Command inherits environment variables from request
- Exit code captured and reported

**Test Strategy**: Integration test with known commands
**Risk**: Medium - Command execution must be reliable

---

#### REQ-EXE-LOC-002: Workspace Isolation
**Priority**: Critical
**GAMP Category**: GxP Critical

**Requirement**:
The system SHALL create a unique, isolated workspace directory for each execution using the execution ID.

**Rationale**:
Prevents cross-contamination between executions and enables audit trail correlation.

**Acceptance Criteria**:
- Workspace path: `{workspace_base}/{execution_id}/`
- Execution ID used for directory name
- Directory created before file injection
- Directory isolated from other executions

**Test Strategy**: Execute multiple concurrent requests, verify separate workspaces
**Risk**: High - Data integrity depends on isolation

**CFR 21 Part 11 Link**: §11.10(a) - Validation of systems to ensure accuracy, reliability

---

#### REQ-EXE-LOC-003: Execution ID Generation
**Priority**: Critical
**GAMP Category**: GxP Critical

**Requirement**:
The system SHALL generate a unique execution ID using UUID v4 if not provided by client.

**Rationale**:
Ensures every execution has a unique identifier for audit trail correlation.

**Acceptance Criteria**:
- If `execution_id` provided in request, use it
- If `execution_id` empty, generate UUID v4
- ID must be filesystem-safe (no `.`, `/`, `..`)
- ID returned in all execution events

**Test Strategy**:
- Test with client-provided ID
- Test with empty ID (auto-generation)
- Test invalid ID rejection

**Risk**: Critical - Audit trail depends on unique IDs

**CFR 21 Part 11 Link**: §11.10(e) - Use of secure, computer-generated, time-stamped audit trails

---

#### REQ-EXE-LOC-004: Output Streaming
**Priority**: Critical
**GAMP Category**: GxP Critical

**Requirement**:
The system SHALL stream stdout and stderr from executing commands in real-time to the client.

**Rationale**:
Provides immediate feedback and enables monitoring of long-running executions.

**Acceptance Criteria**:
- Stdout lines streamed as `EventStdout` events
- Stderr lines streamed as `EventStderr` events
- Line numbers sequential and accurate
- No line loss or reordering

**Test Strategy**: Execute command with known output, verify all lines received
**Risk**: Medium - Output must be complete and ordered

**CFR 21 Part 11 Link**: §11.10(a) - System validation for accuracy

---

### FILE-LOC: Local File Management

#### REQ-FILE-LOC-001: File Injection
**Priority**: Critical
**GAMP Category**: GxP Critical

**Requirement**:
The system SHALL write all files from the execution request to the workspace directory before command execution.

**Rationale**:
Enables self-contained execution with all required input files.

**Acceptance Criteria**:
- Files written to `{workspace}/{relative_path}`
- File content matches request bytes exactly
- Parent directories created as needed
- Files written atomically (all or none)

**Test Strategy**: Inject known files, verify content and paths
**Risk**: Critical - Data integrity of input files

**CFR 21 Part 11 Link**: §11.10(a) - Validation of data input

---

#### REQ-FILE-LOC-002: Path Traversal Prevention
**Priority**: Critical
**GAMP Category**: GxP Critical

**Requirement**:
The system SHALL reject any file paths that attempt directory traversal (e.g., `../`, absolute paths).

**Rationale**:
Prevents security vulnerability and data contamination outside workspace.

**Acceptance Criteria**:
- Paths containing `..` rejected with error
- Absolute paths rejected with error
- Paths resolved relative to workspace only
- Execution fails immediately on invalid path

**Test Strategy**:
- Attempt injection with `../etc/passwd`
- Attempt injection with `/etc/passwd`
- Verify rejection and error message

**Risk**: Critical - Security and data integrity

**CFR 21 Part 11 Link**: §11.10(d) - Limiting system access to authorized individuals

---

#### REQ-FILE-LOC-003: Artifact Collection
**Priority**: Critical
**GAMP Category**: GxP Critical

**Requirement**:
The system SHALL collect files matching retain patterns using glob matching after command execution.

**Rationale**:
Enables retrieval of execution results for analysis and archival.

**Acceptance Criteria**:
- Glob patterns evaluated against workspace contents
- Matching files streamed as `FileChunk` events
- File content transmitted completely
- File count reported in completion event

**Test Strategy**:
- Create files matching patterns
- Verify all matches collected
- Verify non-matches ignored

**Risk**: High - Critical for capturing results

**CFR 21 Part 11 Link**: §11.10(c) - Accurate reproduction of records

---

#### REQ-FILE-LOC-004: Workspace Cleanup
**Priority**: Medium
**GAMP Category**: Non-GxP

**Requirement**:
The system SHALL remove workspace directory after execution if no retain patterns specified.

**Rationale**:
Prevents disk space exhaustion from temporary workspaces.

**Acceptance Criteria**:
- If `retain` empty, workspace deleted after execution
- If `retain` specified, workspace preserved
- Cleanup happens after artifact collection
- Cleanup failure logged but doesn't fail execution

**Test Strategy**:
- Execute without retain, verify cleanup
- Execute with retain, verify preservation

**Risk**: Low - Resource management

---

#### REQ-FILE-LOC-005: Recursive Glob Pattern Support
**Priority**: Critical
**GAMP Category**: GxP Critical

**Requirement**:
The system SHALL support globstar (`**`) retain patterns that match files
recursively across directory levels (e.g. `output/**/*.json`), in addition to
standard single-star (`*`) globbing. Matching behavior SHALL be identical across
the local and Docker execution modes.

**Rationale**:
Clients orchestrating PKPD runs cannot predict the depth of result directories.
Recursive globbing lets a single retain pattern capture nested artifacts without
enumerating every subdirectory, simplifying client orchestration.

**Acceptance Criteria**:
- `**` matches zero or more intermediate path segments
- Single-star `*` continues to match within a single path segment only
- Nested files selected by `**` are streamed as `FileChunk` events
- Non-matching files are ignored
- Local and Docker modes produce equivalent results for the same patterns
- Config `retain_paths` / `commands` override patterns also support `**`

**Test Strategy**:
- Local: inject nested files, retain `out/**/*.json`, verify recursive matches
  and non-matches (`TestLocalExecutorGlobstarCollection`)
- Docker: filter an in-memory container tar with globstar patterns, verifying
  prefix stripping and recursive matching without a daemon
  (`TestMatchTarArtifactsGlobstar`)
- Config: verify `PathOverride`/`CommandOverride` globstar matching
  (`TestOverrideGlobstarMatch`)

**Risk**: High - Determines which results are returned to the client

**CFR 21 Part 11 Link**: §11.10(c) - Accurate reproduction of records

---

### AUD-LOC: Audit Trail (Local Mode)

#### REQ-AUD-LOC-001: Execution Event Correlation
**Priority**: Critical
**GAMP Category**: GxP Critical

**Requirement**:
The system SHALL include the execution ID in every execution event streamed to the client.

**Rationale**:
Enables correlation of all events back to the initiating request and audit entry.

**Acceptance Criteria**:
- `execution_id` field populated in every `ExecutionEvent`
- ID consistent across all events for single execution
- ID matches request `execution_id` or generated UUID

**Test Strategy**: Execute and verify all events contain same execution ID
**Risk**: Critical - Audit trail integrity

**CFR 21 Part 11 Link**: §11.10(e) - Use of audit trails to independently record activities

---

#### REQ-AUD-LOC-002: Timestamping
**Priority**: Critical
**GAMP Category**: GxP Critical

**Requirement**:
The system SHALL include Unix epoch timestamp in every execution event.

**Rationale**:
Provides chronological record of execution activities.

**Acceptance Criteria**:
- `timestamp` field populated in every `ExecutionEvent`
- Timestamp reflects event occurrence time
- Timestamps monotonically increasing (or equal) within execution

**Test Strategy**: Verify timestamps present and chronological
**Risk**: Critical - Audit trail integrity

**CFR 21 Part 11 Link**: §11.10(e) - Time-stamped audit trails

---

#### REQ-AUD-LOC-003: Command Traceability
**Priority**: Critical
**GAMP Category**: GxP Critical

**Requirement**:
The system SHALL log the actual command executed, including any overrides applied.

**Rationale**:
Audit trail must show what actually executed, not just what was requested.

**Acceptance Criteria**:
- Original command from request logged
- Overridden command logged if override applied
- Override pattern and target logged
- Execution ID links command to events

**Test Strategy**:
- Execute with command override
- Verify both original and actual command logged

**Risk**: Critical - Audit trail completeness

**CFR 21 Part 11 Link**: §11.10(e) - Audit trail documenting operator actions

---

#### REQ-AUD-LOC-004: Execution Completion Recording
**Priority**: Critical
**GAMP Category**: GxP Critical

**Requirement**:
The system SHALL report execution completion with exit code, runtime, and files collected.

**Rationale**:
Provides definitive record of execution outcome.

**Acceptance Criteria**:
- `ExecutionComplete` event sent on success
- Exit code captured from process
- Runtime in seconds calculated and reported
- File count accurate

**Test Strategy**:
- Execute successful command, verify exit code 0
- Execute failing command, verify non-zero exit code

**Risk**: Critical - Execution outcome must be accurate

**CFR 21 Part 11 Link**: §11.10(a) - System validation for accuracy and reliability

---

### CFG-LOC: Configuration (Local Mode)

#### REQ-CFG-LOC-001: Mode Selection
**Priority**: Critical
**GAMP Category**: GxP Critical

**Requirement**:
The system SHALL support selection of executor mode via configuration or command-line flag.

**Rationale**:
Enables deployment-specific execution strategies.

**Acceptance Criteria**:
- `executor.mode` in YAML config honored
- `--mode` flag overrides config
- Valid modes: `local`, `docker`, `file-server`, `scheduler`
- Invalid mode causes startup failure

**Test Strategy**:
- Start with each valid mode
- Attempt invalid mode, verify error

**Risk**: Medium - Incorrect mode could bypass controls

---

#### REQ-CFG-LOC-002: Workspace Base Configuration
**Priority**: Medium
**GAMP Category**: GxP Critical

**Requirement**:
The system SHALL allow configuration of workspace base directory.

**Rationale**:
Enables control over workspace location for disk space and access control.

**Acceptance Criteria**:
- `executor.local.workspace_base` in config honored
- `--workspace` flag overrides config
- Empty value defaults to system temp directory
- Invalid directory causes startup failure

**Test Strategy**: Configure custom workspace, verify usage
**Risk**: Medium - Workspace location affects data retention

---

#### REQ-CFG-LOC-003: Configuration Validation
**Priority**: Critical
**GAMP Category**: GxP Critical

**Requirement**:
The system SHALL validate configuration at startup and fail fast on errors.

**Rationale**:
Prevents operation with invalid configuration.

**Acceptance Criteria**:
- Invalid executor mode rejected
- Invalid port rejected
- Path traversal in overrides rejected
- Validation errors descriptive

**Test Strategy**: Provide invalid config, verify rejection
**Risk**: High - Invalid config could cause unexpected behavior

**CFR 21 Part 11 Link**: §11.10(a) - System validation

---

### SEC-LOC: Security (Local Mode)

#### REQ-SEC-LOC-001: Command Execution Isolation
**Priority**: Critical
**GAMP Category**: GxP Critical

**Requirement**:
The system SHALL execute commands with the same user permissions as the server process.

**Rationale**:
Local mode does not provide additional isolation; documentation and access control required.

**Acceptance Criteria**:
- Command executes as server user
- No privilege escalation
- No additional sandboxing applied
- Documented security limitation

**Test Strategy**: Execute command, verify process owner
**Risk**: Critical - Security posture must be understood

**CFR 21 Part 11 Link**: §11.10(d) - Limiting system access

---

#### REQ-SEC-LOC-002: Path Sanitization
**Priority**: Critical
**GAMP Category**: GxP Critical

**Requirement**:
The system SHALL sanitize all file paths to prevent directory traversal.

**Rationale**:
Prevents unauthorized file access or modification.

**Acceptance Criteria**:
- Paths cleaned with `filepath.Clean()`
- Relative path validation
- Path traversal sequences rejected
- Validation before any file operations

**Test Strategy**: See REQ-FILE-LOC-002
**Risk**: Critical - Data integrity and security

---

### ERR-LOC: Error Handling (Local Mode)

#### REQ-ERR-LOC-001: Execution Errors
**Priority**: Critical
**GAMP Category**: GxP Critical

**Requirement**:
The system SHALL report execution errors via `ExecutionError` event with descriptive message.

**Rationale**:
Client must know when execution fails and why.

**Acceptance Criteria**:
- Command not found → error event
- Permission denied → error event
- File injection failure → error event
- Error message descriptive and actionable

**Test Strategy**:
- Execute non-existent command
- Inject file to read-only location
- Verify error events

**Risk**: Medium - Error reporting affects troubleshooting

---

#### REQ-ERR-LOC-002: Graceful Degradation
**Priority**: Medium
**GAMP Category**: Non-GxP

**Requirement**:
The system SHALL continue artifact collection even if command exits with non-zero code.

**Rationale**:
Partial results may still be valuable for diagnostics.

**Acceptance Criteria**:
- Non-zero exit code doesn't prevent artifact collection
- Artifacts collected and streamed normally
- Completion event includes actual exit code

**Test Strategy**: Execute failing command, verify artifacts collected
**Risk**: Low - Convenience feature

---

## File Server Mode Requirements

### FILE-SRV: File Server Mode

#### REQ-FILE-SRV-001: File Serving (Future)
**Priority**: Critical
**GAMP Category**: GxP Critical
**Status**: Not Implemented

**Requirement**:
The system SHALL support file-server mode that serves files from existing workspaces without command execution.

**Rationale**:
Enables artifact collection after K8s batch job completion.

**Acceptance Criteria**:
- `GetFiles` RPC implemented
- Workspace path validated
- Glob patterns matched
- Files streamed as chunks

**Test Strategy**: TBD
**Risk**: Critical - Required for K8s integration

**CFR 21 Part 11 Link**: §11.10(c) - Accurate record reproduction

---

#### REQ-FILE-SRV-002: Workspace Validation (Future)
**Priority**: Critical
**GAMP Category**: GxP Critical
**Status**: Not Implemented

**Requirement**:
The system SHALL validate workspace path exists and is accessible before file collection.

**Rationale**:
Prevents errors and ensures data availability.

**Acceptance Criteria**:
- Workspace directory must exist
- Workspace must be readable
- Path traversal prevented
- Validation errors reported

**Test Strategy**: TBD
**Risk**: Medium - Error handling

---

## Cross-Cutting Requirements

### VAL-ALL: Validation

#### REQ-VAL-ALL-001: Test Traceability
**Priority**: Critical
**GAMP Category**: GxP Critical

**Requirement**:
The system SHALL provide mechanism to trace tests back to requirements.

**Rationale**:
Enables validation documentation and requirement coverage analysis.

**Acceptance Criteria**:
- Test metadata includes requirement ID
- Test names indicate requirement
- Coverage report shows requirement mapping
- Automated validation test suite

**Test Strategy**: Review test codebase for metadata
**Risk**: High - Validation depends on traceability

**CFR 21 Part 11 Link**: §11.10(a) - Validation of systems

---

#### REQ-VAL-ALL-002: Test Boundaries
**Priority**: Medium
**GAMP Category**: GxP Critical

**Requirement**:
Tests SHALL validate that commands are called correctly without executing actual computations.

**Rationale**:
Per CLAUDE.md: "Boundaries for most testing should limit to 'Are we calling the correct thing'"

**Acceptance Criteria**:
- Unit tests verify command construction
- Integration tests verify command invocation
- Black box tests verify file content requirements
- Actual computation not tested (external to Hermes)

**Test Strategy**: Mock command execution where appropriate
**Risk**: Medium - Test scope definition

---

## Requirement Traceability Matrix

| Requirement ID | Category | Priority | Implementation Status | Test Status |
|---------------|----------|----------|---------------------|-------------|
| REQ-EXE-LOC-001 | Execution | Critical | ✅ Implemented | ⏳ Pending |
| REQ-EXE-LOC-002 | Execution | Critical | ✅ Implemented | ⏳ Pending |
| REQ-EXE-LOC-003 | Execution | Critical | ✅ Implemented | ⏳ Pending |
| REQ-EXE-LOC-004 | Execution | Critical | ✅ Implemented | ⏳ Pending |
| REQ-FILE-LOC-001 | File Mgmt | Critical | ✅ Implemented | ⏳ Pending |
| REQ-FILE-LOC-002 | Security | Critical | ✅ Implemented | ⏳ Pending |
| REQ-FILE-LOC-003 | File Mgmt | Critical | ✅ Implemented | ⏳ Pending |
| REQ-FILE-LOC-004 | File Mgmt | Medium | ✅ Implemented | ⏳ Pending |
| REQ-FILE-LOC-005 | File Mgmt | Critical | ✅ Implemented | ✅ Tested |
| REQ-AUD-LOC-001 | Audit | Critical | ✅ Implemented | ⏳ Pending |
| REQ-AUD-LOC-002 | Audit | Critical | ✅ Implemented | ⏳ Pending |
| REQ-AUD-LOC-003 | Audit | Critical | ⚠️ Partial | ⏳ Pending |
| REQ-AUD-LOC-004 | Audit | Critical | ✅ Implemented | ⏳ Pending |
| REQ-CFG-LOC-001 | Config | Critical | ✅ Implemented | ⏳ Pending |
| REQ-CFG-LOC-002 | Config | Medium | ✅ Implemented | ⏳ Pending |
| REQ-CFG-LOC-003 | Config | Critical | ✅ Implemented | ⏳ Pending |
| REQ-SEC-LOC-001 | Security | Critical | ✅ Implemented | ⏳ Pending |
| REQ-SEC-LOC-002 | Security | Critical | ✅ Implemented | ⏳ Pending |
| REQ-ERR-LOC-001 | Error | Critical | ✅ Implemented | ⏳ Pending |
| REQ-ERR-LOC-002 | Error | Medium | ✅ Implemented | ⏳ Pending |
| REQ-FILE-SRV-001 | File Server | Critical | ❌ Future | ❌ N/A |
| REQ-FILE-SRV-002 | File Server | Critical | ❌ Future | ❌ N/A |
| REQ-VAL-ALL-001 | Validation | Critical | ⏳ In Progress | ⏳ Pending |
| REQ-VAL-ALL-002 | Validation | Medium | ⏳ In Progress | ⏳ Pending |

**Legend**:
- ✅ Implemented and working
- ⚠️ Partially implemented
- ⏳ In progress or pending
- ❌ Not started / future

---

## Test Metadata Format

Tests should include requirement traceability in comments:

```go
// TestLocalExecutorWorkspaceIsolation validates REQ-EXE-LOC-002
// Requirement: Workspace Isolation
// Priority: Critical
// Category: GxP Critical
func TestLocalExecutorWorkspaceIsolation(t *testing.T) {
    // Test implementation
}
```

---

## Validation Test Suite

A validation test suite should be created that:
1. Runs all tests tagged with requirement IDs
2. Generates traceability report
3. Reports requirement coverage
4. Suitable for GxP validation packages

Example GitHub Action:
```yaml
name: Validation Tests
on: [release]
jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - name: Run validation tests
        run: go test -v -tags=validation ./...
      - name: Generate traceability report
        run: ./scripts/generate-traceability.sh
```

---

## Change Control

**Document Version**: 1.0
**Last Updated**: 2025-01-25
**Author**: Hermes Development Team
**Approval**: Pending

### Change History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-01-25 | AI/Human Pair | Initial requirements for local and file-server modes |

---

## References

1. CFR 21 Part 11 - Electronic Records; Electronic Signatures
2. GAMP 5 Guide - Risk-Based Approach to Compliant GxP Computerized Systems
3. Hermes Design Document (design.md)
4. Hermes Development Guidelines (CLAUDE.md)
5. ICH Q9 Quality Risk Management
6. Data Integrity Guidance (FDA, MHRA)
