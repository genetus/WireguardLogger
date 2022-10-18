package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"os/exec"
	"strings"
)

var emailConfig = readFile("email_config")

func main() {
	var data, err = exec.Command("wg", "show").Output()
	var previousRun = readFile("previous_run")
	var workString = strings.Split(string(data), "\n")

	var users = readFile("watching_peers")
	var emailsForSend = readFile("emails")
	var currentPeer = ""
	var statusString = ""
	var realName = ""
	for i := 0; i < len(workString); i++ {
		if strings.Contains(workString[i], "peer") {
			realName = ""
			peerName := strings.Split(workString[i], ": ")[1]
			fmt.Println("Peer found:", peerName)
			if peerName != currentPeer {
				currentPeer = peerName
			}
			var ok bool
			realName, ok = users[peerName]
			if ok {
				fmt.Println("Peer user is", realName)
			}
		}
		if strings.Contains(workString[i], "latest handshake:") {
			status := "offline"
			if parseOnlineStatus(workString[i], 5) {
				fmt.Println("Current peer is online")
				status = "online"
			} else {
				fmt.Println("Current peer is offline")
			}

			statusString = statusString + currentPeer + " " + status + "\n"

			previousStatus, ok := previousRun[currentPeer]
			if ok {
				if previousStatus == status {
					fmt.Println("User is not change its status")
				} else if previousStatus == "online" {
					fmt.Println("User disconnected")
					if realName != "" {
						for email, emailName := range emailsForSend {
							sendEmail(realName, emailName, email, "disconnected")
						}
					}
				} else if previousStatus == "offline" || previousStatus == "" {
					fmt.Println("User connected")
					if realName != "" {
						for email, emailName := range emailsForSend {
							sendEmail(realName, emailName, email, "connected")
						}
					}
				}
			}
		}
	}

	err = os.WriteFile("/etc/WireguardLogger/previous_run", []byte(statusString), 0644)
	if err != nil {
		panic(err)
	}

}

func parseOnlineStatus(status string, minutesToOffline int) bool {
	if strings.Contains(status, "hour") || strings.Contains(status, "day") || strings.Contains(status, "week") {
		return false
	}
	if !strings.Contains(status, "minute") {
		return true
	}
	var minutes int
	if _, err := fmt.Sscanf(strings.Split(status, ": ")[1], "%d", &minutes); err == nil {
		return minutes <= minutesToOffline
	}
	return false
}

func readFile(fileName string) map[string]string {
	var data = make(map[string]string)
	fileName = "/etc/WireguardLogger/" + fileName

	previousData, err := os.ReadFile(fileName)
	if err != nil {
		log.Println("No file", fileName, "It is normal just skip")
	} else {
		previousString := strings.Split(string(previousData), "\n")
		for i := 0; i < len(previousString); i++ {
			if previousString[i] == "" {
				continue
			}
			currentData := strings.Split(previousString[i], " ")
			left, right := currentData[0], currentData[1:]
			data[left] = strings.Join(right, " ")
		}
	}

	return data
}

func sendEmail(whoConnected string, name string, email string, action string) {

	fmt.Println("Sending a email to", email, ", because user", whoConnected, action, "to VPN")

	from := mail.Address{emailConfig["fromName"], emailConfig["fromEmail"]}
	to := mail.Address{name, email}
	subj := whoConnected + " " + action + " to VPN"
	body := whoConnected + " " + action + " to VPN.\nWith best regard, Noveo Team."

	// Setup headers
	headers := make(map[string]string)
	headers["From"] = from.String()
	headers["To"] = to.String()
	headers["Subject"] = subj

	// Setup message
	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + body

	// Connect to the SMTP Server
	servername := emailConfig["smtp"]

	host, _, _ := net.SplitHostPort(servername)
	password := emailConfig["password"]

	auth := smtp.PlainAuth("", emailConfig["login"], password, host)

	// TLS config
	tlsconfig := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	}

	// Here is the key, you need to call tls.Dial instead of smtp.Dial
	// for smtp servers running on 465 that require an ssl connection
	// from the very beginning (no starttls)
	conn, err := tls.Dial("tcp", servername, tlsconfig)
	if err != nil {
		log.Panic(err)
	}

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		log.Panic(err)
	}

	// Auth
	if err = c.Auth(auth); err != nil {
		log.Panic(err)
	}

	// To && From
	if err = c.Mail(from.Address); err != nil {
		log.Panic(err)
	}

	if err = c.Rcpt(to.Address); err != nil {
		log.Panic(err)
	}

	// Data
	w, err := c.Data()
	if err != nil {
		log.Panic(err)
	}

	_, err = w.Write([]byte(message))
	if err != nil {
		log.Panic(err)
	}

	err = w.Close()
	if err != nil {
		log.Panic(err)
	}

	err = c.Quit()
	if err != nil {
		log.Panic(err)
	}
}
