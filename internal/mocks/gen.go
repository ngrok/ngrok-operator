package mocks

// Note: Generate the mock files with names like mock_*.go. This is so that
// the generated files are picked up by the .gitattributes file.

//go:generate go tool go.uber.org/mock/mockgen -package mocks -destination mock_conn.go net Conn
