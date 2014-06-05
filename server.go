package main

import (
    "bufio"
    "errors"
    "fmt"
    "io"
    "net"
    "os"
    "strings"
    "time"
)

const (
    // CLIENT_TIMEOUT how long a client has before the connection times out
    CLIENT_TIMEOUT time.Duration = 100 * time.Second

    // SMTP_MAX_SIZE the max number of characters a client can send as DATA
    SMTP_MAX_DATA_SIZE int = 131072
)

var (
    sem chan int // currently active clients
)

type Client struct {
    Conn        net.Conn
    BufIn       *bufio.Reader
    BufOut      *bufio.Writer
    Helo        string
    MailFrom    string
    MailTo      string
    Response    string
    State       int
}

func main() {
    // start with max pool of 10 users
    sem = make(chan int, 10)

    // start listening for tcp connections
    listener, err := net.Listen("tcp", "0.0.0.0:8888")
    if err != nil {
        fmt.Printf("Cannot listen on port 8888, %v", err)
    } else {
        fmt.Println("Listening on 0.0.0.0:8888")
    }
    for {
        conn, err := listener.Accept()
        if err != nil {
            fmt.Printf("Accept error: %v", err)
            continue
        }
        // acquire a lock on the semaphore
        sem <- 1
        go handleClient(&Client{
            Conn: conn,
            BufIn: bufio.NewReader(conn),
            BufOut: bufio.NewWriter(conn),
        })
    }
}

func handleClient(client *Client) {
    var response string

    defer closeConnection(client.Conn)

    hostname, _ := os.Hostname()

    for i := 0; i < 100; i++ {
        switch client.State {
        // acknowledge that the connection has been established
        case 0:
            response = formatResponse("220 "+hostname+" ready")
            client.State = 1
        // read from buffer for standard SMTP commands
        case 1:
            input, err := readClientInput(client)
            if err != nil {
                fmt.Println("Read error: %v", err)
                if err == io.EOF {
                    // client closed the connection
                    return
                }
                if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
                    // too slow, timeout
                    return
                }
            }
            input = strings.Trim(input, "\r\n")
            cmd := strings.ToUpper(input)
            switch {
            case strings.Index(cmd, "HELO") == 0:
                if len(input) > 5 {
                    client.Helo = input[5:]
                }
                response = formatResponse("250 "+hostname+" Hello")
            case strings.Index(cmd, "EHLO") == 0:
                if len(input) > 5 {
                    client.Helo = input[5:]
                }
                response = formatResponse("250 "+hostname+" Hello")
            case strings.Index(cmd, "MAIL FROM:") == 0:
                if len(input) > 10 {
                    client.MailFrom = input[10:]
                }
                response = formatResponse("250 OK")
            case strings.Index(cmd, "RCPT TO:") == 0:
                if len(input) > 8 {
                    client.MailTo = input[8:]
                }
                response = formatResponse("250 Accepted")
            case strings.Index(cmd, "NOOP") == 0:
                response = formatResponse("250 OK")
            case strings.Index(cmd, "DATA") == 0:
                response = formatResponse("354 Enter message, ending with \".\" on a line by itself")
                client.State = 2
            case strings.Index(cmd, "QUIT") == 0:
                response = formatResponse("221 Bye")
                return
            default:
                response = formatResponse("500 unrecognized command")
            }
        // read in DATA
        case 2:
            data, err := readClientInput(client)
            if err != nil {
                fmt.Println("DATA read error: %v", err)
            }
            response = formatResponse("250 OK")
            fmt.Printf("Received DATA %s", data)
            // read input from STDIN again
            client.State = 1
        }
        // send back a response to the client
        err := responseWrite(client.BufOut, response)
        if err != nil {
            if err == io.EOF {
                // client closed the connection
                return
            }
            if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
                // too slow, timeout
                return
            }
        }
    }
}

func formatResponse(response string) string {
    return response + "\r\n"
}

func closeConnection(conn net.Conn) {
    conn.Close()
    // release the semaphore back to the pool
    <-sem
}

func responseWrite(buffer *bufio.Writer, response string) error {
    _, err := buffer.WriteString(response)
    buffer.Flush()
    return err
}

func readClientInput(client *Client) (input string, err error) {
    // default state terminator
    terminator := "\r\n"
    if client.State == 2 {
        // DATA state
        terminator = "\r\n.\r\n"
    }
    for err == nil {
        // set a timeout for the client
        client.Conn.SetDeadline(time.Now().Add(CLIENT_TIMEOUT))
        response, err := readInput(client.BufIn)
        if response != "" {
            input += response

            if len(input) > SMTP_MAX_DATA_SIZE {
                err = errors.New("Maximum DATA size exceeded (" +
                    string(SMTP_MAX_DATA_SIZE) + ")")
                return input, err
            }
        }
        if strings.HasSuffix(input, terminator) {
            break
        }
    }
    return input, err
}

func readInput(buffer *bufio.Reader) (string, error) {
    input, err := buffer.ReadString('\n')
    if err != nil {
        return "", err
    }
    return input, err
}
