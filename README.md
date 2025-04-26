# jsreader

A powerful Go-based tool that analyzes JavaScript files to discover endpoints, API URLs, tokens, and other sensitive information that might be exposed in client-side code. Special thanks for Timoria and RIGOLIT.

## Features

jsreader can detect various types of sensitive information:

- **S3 Buckets** - Potential public AWS S3 bucket URLs
- **Firebase Resources**:
  - Firebase Database URLs
  - Firebase Storage URLs
  - Firebase Application URLs
- **API Endpoints**:
  - General API endpoints
  - API versions
  - API subdomains
  - API components
- **GraphQL Endpoints** - GraphQL API endpoints
- **Authentication Endpoints** - Authentication, OAuth, and user-related endpoints
- **Telegram Bot Tokens** - Exposed Telegram API tokens
- **URLs in Variables** - URLs stored in JavaScript variables

## Installation

### Using Go Install

```bash
go install github.com/number731/jsreader@latest
```

### Manual Installation

```bash
git clone https://github.com/number731/jsreader.git

# Change to the project directory
cd jsreader

# Build the executable
go build -o jsreader.go

# Optional: Move to a directory in your PATH
sudo mv jsreader /usr/local/bin/
```

## Usage

JS Endpoint Finder provides several options for analyzing JavaScript files:

```
Usage:
  jsreader [options]

Options:
  -t int     Number of threads to use (default 1)
  -i string  Path to file with list of JS URLs (one per line)
  -f string  Path to single JS file to analyze
  -p         Enable pipe mode (read from stdin)
  -o string  Output file to save results (.txt)
```

### Examples

**Analyze a single local JavaScript file:**
```bash
jsreader -f /path/to/script.js
```

**Analyze a remote JavaScript file:**
```bash
jsreader -f https://example.com/script.js
```

**Process multiple JavaScript files from a list:**
```bash
jsreader -i urls.txt -t 5
```

**Process data from pipe:**
```bash
cat urls.txt | jsreader -p
```

**Save results to a file:**
```bash
jsreader -i urls.txt -o results.txt
```

**Combine with other tools:**
```bash
cat domains.txt | katana -d 5 -jc | grep '\.js$' | jsreader -p

```

## Example Output

When running JS Endpoint Finder, results will be color-coded for easy identification:

- **Red:** S3 Buckets
- **Yellow:** Firebase resources
- **Green:** API endpoints
- **Cyan:** GraphQL endpoints
- **Purple:** Authentication endpoints
- **Blue:** URLs in variables
- **Orange:** Telegram tokens
- **Teal:** API subdomains
- **Pink:** API versions
- **Magenta:** API components

Example output:
```
[API] https://api.example.com/v1/users
   Details: API endpoint - investigate available methods
   Source: https://example.com/main.js

[Firebase DB] https://my-app.firebaseio.com/data
   Details: Firebase service - check security rules
   Source: https://example.com/main.js

[S3 Bucket] https://assets.s3-us-west-2.amazonaws.com/images
   Details: Potential public S3 bucket - check permissions
   Source: https://example.com/main.js
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.

