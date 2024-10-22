# Connectivity Tester

Connectivity Tester is a tool for managing and testing server connections. It allows you to add servers to a database and perform connectivity tests on them.

## Features

- Add servers to a database from a file
- Test connectivity of all servers or selectively retest based on previous errors
- Support for both TCP and UDP testing

## Prerequisites

- Go 1.17 or higher
- PostgreSQL database

## Installation

1. Clone the repository:
   ```
   git clone https://github.com/yourusername/connectivity-tester.git
   ```
2. Navigate to the project directory:
   ```
   cd connectivity-tester
   ```
3. Install dependencies:
   ```
   go mod tidy
   ```

## Configuration

Create a `config.yaml` file in the project root with the following structure:

```yaml
database:
  host: localhost
  port: 5432
  user: your_username
  password: your_password
  dbname: your_database_name
  sslmode: disable

ipinfo:
  token: your_ipinfo_token

connectivity:
  resolver: 8.8.8.8
  domain: example.com
```

## Usage

### Adding Servers

To add servers from a file:

```
go run main.go add-servers path/to/your/file.txt
```

### Testing Servers

- To test all servers:
  ```
  go run main.go test-servers
  ```

- To retest only servers with TCP errors (excluding 'connect' errors):
  ```
  go run main.go test-servers --tcp
  ```

- To retest only servers with UDP errors:
  ```
  go run main.go test-servers --udp
  ```

- To retest servers with either TCP or UDP errors:
  ```
  go run main.go test-servers --tcp --udp
  ```

## Debug Mode

To enable debug logging, add the `-d` or `--debug` flag to any command:

```
go run main.go -d test-servers
```

## License

[Your chosen license]

## Contributing

[Your contribution guidelines]