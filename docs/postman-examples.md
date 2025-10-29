# Postman Request Examples with Test Data

This document provides ready-to-use request bodies for testing Hermes with Postman.

## Simple Echo Test

Copy and paste this directly into Postman:

```json
{
  "command": "echo",
  "args": ["Hello from Hermes!"],
  "execution_id": "test-echo-001"
}
```

**Expected Output:**
- Started event
- Stdout: "Hello from Hermes!"
- Complete event with exit_code: 0

---

## Test with File Creation and Retention

Creates a simple text file and retrieves it:

```json
{
  "command": "bash",
  "args": ["-c", "echo 'Test output content' > output.txt && cat output.txt"],
  "working_dir": "/workspace",
  "retain": ["output.txt"],
  "execution_id": "test-file-retention"
}
```

**Expected Output:**
- Started event
- Stdout: "Test output content"
- FileChunk event with base64-encoded file content
- Complete event

---

## Test with Multiple Files

```json
{
  "command": "bash",
  "args": [
    "-c",
    "echo 'File 1 content' > file1.txt && echo 'File 2 content' > file2.log && echo 'File 3 content' > file3.csv"
  ],
  "working_dir": "/workspace",
  "retain": ["*.txt", "*.log", "*.csv"],
  "execution_id": "test-multiple-files"
}
```

**Expected Output:**
- FileChunk events for file1.txt, file2.log, file3.csv
- Each file streamed separately

---

## Test with Environment Variables

```json
{
  "command": "bash",
  "args": ["-c", "echo \"MY_VAR is: $MY_VAR\" && echo \"ANOTHER_VAR is: $ANOTHER_VAR\""],
  "environment": {
    "MY_VAR": "test_value",
    "ANOTHER_VAR": "another_test"
  },
  "execution_id": "test-env-vars"
}
```

**Expected Output:**
```
MY_VAR is: test_value
ANOTHER_VAR is: another_test
```

---

## Test with File Injection

This injects a simple text file into the workspace:

```json
{
  "command": "bash",
  "args": ["-c", "ls -la /workspace && cat /workspace/injected.txt"],
  "working_dir": "/workspace",
  "files": {
    "injected.txt": "VGhpcyBpcyBhIHRlc3QgZmlsZSBpbmplY3RlZCBieSBIZXJtZXMh"
  },
  "execution_id": "test-file-injection"
}
```

**File Content Decoded:**
The base64 string decodes to: "This is a test file injected by Hermes!"

**Expected Output:**
- List of files including injected.txt
- Content of injected.txt printed

---

## Test with Resource Limits

```json
{
  "command": "bash",
  "args": ["-c", "echo 'Starting...' && sleep 5 && echo 'Done!'"],
  "limits": {
    "cpu_limit": "1",
    "memory_limit": "256M",
    "timeout_seconds": 10
  },
  "execution_id": "test-limits"
}
```

**Expected Output:**
- Should complete within 10 seconds
- If timeout is reduced to 3 seconds, should fail with timeout error

---

## NONMEM Test (Minimal Example)

This assumes you have a NONMEM container and license. Replace the base64 strings with actual encoded files:

```json
{
  "command": "nonmem",
  "args": ["control.mod", "control.lst"],
  "working_dir": "/workspace",
  "files": {
    "control.mod": "<YOUR_BASE64_ENCODED_CONTROL_FILE>",
    "nonmem.lic": "<YOUR_BASE64_ENCODED_LICENSE>"
  },
  "retain": [
    "*.lst",
    "*.ext",
    "*.cov",
    "*.cor",
    "*.coi",
    "*.phi",
    "FDATA",
    "PRDERR"
  ],
  "environment": {
    "NMLICENSE": "/workspace/nonmem.lic"
  },
  "container_image": "pharmalytica/nonmem:nm75",
  "limits": {
    "cpu_limit": "4",
    "memory_limit": "8G",
    "timeout_seconds": 3600
  },
  "execution_id": "nonmem-test-001"
}
```

---

## Encoding Files for Testing

### Using Bash

```bash
# Encode a file
base64 -w 0 myfile.txt

# Encode inline text
echo -n "This is my test content" | base64
```

### Using Python

```python
import base64

# Encode a file
with open('model.mod', 'rb') as f:
    encoded = base64.b64encode(f.read()).decode('utf-8')
    print(encoded)

# Encode a string
text = "This is my test content"
encoded = base64.b64encode(text.encode()).decode('utf-8')
print(encoded)
```

### Using Node.js

```javascript
const fs = require('fs');

// Encode a file
const fileContent = fs.readFileSync('model.mod');
const encoded = fileContent.toString('base64');
console.log(encoded);

// Encode a string
const text = "This is my test content";
const encoded = Buffer.from(text).toString('base64');
console.log(encoded);
```

---

## Quick Reference: Common Base64-Encoded Strings

For quick testing, here are some pre-encoded strings:

| Decoded Text | Base64 Encoded |
|--------------|----------------|
| `Hello, World!` | `SGVsbG8sIFdvcmxkIQ==` |
| `Test file content` | `VGVzdCBmaWxlIGNvbnRlbnQ=` |
| `This is a test` | `VGhpcyBpcyBhIHRlc3Q=` |
| `NONMEM test data` | `Tk9OTUVNIHRlc3QgZGF0YQ==` |
| `Line 1\nLine 2\nLine 3` | `TGluZSAxCkxpbmUgMgpMaW5lIDM=` |

---

## Testing Command Overrides

To test that your command overrides are working:

### Test 1: Verify Mapping Happens

```json
{
  "command": "bin/nonmem",
  "args": ["--version"],
  "execution_id": "test-override-001"
}
```

With server config:
```yaml
overrides:
  commands:
    - pattern: "*/nonmem"
      target: "/opt/NONMEM/nm75/run/nmfe75"
```

**Expected:** Server should map `bin/nonmem` to `/opt/NONMEM/nm75/run/nmfe75`

### Test 2: Check Audit Logs

After running the above, check Hermes logs for audit entries showing the override was applied.

---

## Testing Error Conditions

### Invalid Command

```json
{
  "command": "/nonexistent/command",
  "args": [],
  "execution_id": "test-error-001"
}
```

**Expected:** ExecutionError event with appropriate error message

### Timeout Test

```json
{
  "command": "sleep",
  "args": ["60"],
  "limits": {
    "timeout_seconds": 5
  },
  "execution_id": "test-timeout"
}
```

**Expected:** Execution should be cancelled after 5 seconds with timeout error

### Missing Required File

```json
{
  "command": "cat",
  "args": ["/workspace/nonexistent.txt"],
  "working_dir": "/workspace",
  "execution_id": "test-missing-file"
}
```

**Expected:** Command fails with "No such file or directory" in stderr

---

## Postman Pre-request Scripts

Add this to your request's **Pre-request Script** tab to dynamically generate test data:

### Generate Random Execution ID

```javascript
const timestamp = Date.now();
const random = Math.floor(Math.random() * 1000);
pm.variables.set('execution_id', `test-${timestamp}-${random}`);
```

Then in your request body:
```json
{
  "execution_id": "{{execution_id}}"
}
```

### Encode File Content

```javascript
// Encode a string to base64
const content = "This is my test file content\nLine 2\nLine 3";
const base64Content = btoa(content);
pm.variables.set('file_content_base64', base64Content);
```

Usage:
```json
{
  "files": {
    "test.txt": "{{file_content_base64}}"
  }
}
```

---

## Postman Tests Scripts

Add this to your request's **Tests** tab to validate responses:

### Test Execution Completed Successfully

```javascript
pm.test("Execution completed with exit code 0", function () {
    const messages = pm.response.messages;
    const completeEvent = messages.find(msg => msg.complete);

    pm.expect(completeEvent).to.exist;
    pm.expect(completeEvent.complete.exit_code).to.equal(0);
});
```

### Test Files Were Retained

```javascript
pm.test("Files were retained", function () {
    const messages = pm.response.messages;
    const fileChunks = messages.filter(msg => msg.file_chunk);

    pm.expect(fileChunks.length).to.be.above(0);
});
```

### Test Stdout Contains Expected Text

```javascript
pm.test("Stdout contains expected output", function () {
    const messages = pm.response.messages;
    const stdoutEvents = messages.filter(msg => msg.stdout);
    const allOutput = stdoutEvents.map(e => e.stdout.line).join('\n');

    pm.expect(allOutput).to.include("expected text");
});
```

---

## Collection Runner

To run multiple tests sequentially:

1. Click **Runner** in Postman
2. Select **Hermes gRPC API** collection
3. Choose which requests to run
4. Set iterations and delays
5. Click **Run Hermes gRPC API**

This is useful for regression testing after changes.

---

## Next Steps

1. Import the collection from `hermes.postman_collection.json`
2. Try the simple examples above
3. Modify for your specific use cases
4. Add custom tests and scripts
5. Create environments for different servers

See [POSTMAN_TESTING.md](POSTMAN_TESTING.md) for detailed setup instructions.
