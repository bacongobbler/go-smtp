package main

import (
        "bytes"
        "log"
        "net/smtp"
)

func main() {
        // Connect to the remote SMTP server.
        c, err := smtp.Dial("localhost:8888")
        if err != nil {
                log.Fatalf("Error while connecting: %v", err)
        }
        // Set the sender and recipient.
        c.Mail("bacongobbler@localhost")
        c.Rcpt("test@localhost")
        // Send the email body.
        wc, err := c.Data()
        if err != nil {
                log.Fatal(err)
        }
        defer wc.Close()
        buf := bytes.NewBufferString("test")
        if _, err = buf.WriteTo(wc); err != nil {
                log.Fatal(err)
        }
}
