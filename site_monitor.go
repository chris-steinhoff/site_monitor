package main

import (
	"bytes"
	"crypto/md5"
	"flag"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"regexp"
	"strconv"
	"text/template"
)

func main() {
	const (
		url_default = "<url>"
		file_default = "<path/to/file>"
		filename_default_prefix = "/tmp/site_monitor-"
		filename_default = filename_default_prefix + "url"
	)

	var (
		err error
		url string
		filename string
		smtpConfigFile string
		emailConfigFile string
		ticketsConfigFile string
		file *os.File
		currHash, newHash []byte
		comp int
		smtp SmtpConfig
		email EmailConfig
		tickets EmailConfig
	)

	// Read command line arguments
	flag.StringVar(&url, "url", url_default, "URL to monitor.")
	flag.StringVar(&filename, "file", filename_default, "Download to this file.")
	flag.StringVar(&smtpConfigFile, "smtp", file_default, "SMTP server config file.")
	flag.StringVar(&emailConfigFile, "email", file_default, "Email message config file.")
	flag.StringVar(&ticketsConfigFile, "tickets", file_default,
			"Buy Tickets button email message config file.")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "site_monitor.go -url <url> [-file <filename>]")
		flag.PrintDefaults()
		os.Exit(1)
	}
	flag.Parse()

	// Validate arguments
	if url == url_default {
		flag.Usage()
	}
	/* TODO figure out how to save url-filenames
	if filename == filename_default {
		filename = filename_default_prefix + url
	}*/
	if smtpConfigFile == file_default {
		log.Println("Please supply a smtp config file.")
		flag.Usage()
	}
	if emailConfigFile == file_default {
		log.Println("Please supply an email config file.")
		flag.Usage()
	}
	if ticketsConfigFile == file_default {
		log.Println("Please supply a tickets email config file.")
		flag.Usage()
	}

	// SMTP Configuration
	err = readJsonFile(smtpConfigFile, &smtp)
	if err != nil {
		log.Fatalln(err)
	}
	//log.Printf("smtp: %+v\n", smtp)
	// Change Email Configuration
	err = readJsonFile(emailConfigFile, &email)
	if err != nil {
		log.Fatalln(err)
	}
	log.Printf("email: %+v\n", email)
	// Tickets Email Configuration
	err = readJsonFile(ticketsConfigFile, &tickets)
	if err != nil {
		log.Fatalln(err)
	}
	log.Printf("tickets: %+v\n", tickets)

	// Load the current file
	file, currHash, err = GetFile(filename)
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("Current hash:", currHash)

	// Download the page we're monitoring
	err = Download(file, url)
	if err != nil {
		log.Fatalln(err)
	}

	// Get the new page's hash
	newHash, err = Hash(file)
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("New hash:", newHash)

	// Compare to two hashes
	comp = bytes.Compare(currHash, newHash)
	log.Println("Hash comparison:", comp)

	// If they're different, notify the user of the change
	if comp == 0 {
		log.Println("Unchanged")
	} else {
		log.Println("!!! The page has changed")
		err = SendNotification(smtp, email, url)
		if err != nil {
			log.Fatalln(err)
		}
		var buy, err = hasBuyButton(file)
		if err != nil {
			log.Fatalln(err)
		}
		if buy {
			log.Println("!!!!!! Found the \"Buy Tickets\" button")
			err = SendNotification(smtp, tickets, url)
			if err != nil {
				log.Fatalln(err)
			}
		}
	}
}

func readJsonFile(filename string, v interface{}) error {
	var (
		file *os.File
		dec *json.Decoder
		err error
	)

	// Open the file
	file, err = os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Parse the JSON file
	dec = json.NewDecoder(file)
	err = dec.Decode(v)

	return err
}

func GetFile(filename string) (*os.File, []byte, error) {
	var (
		file *os.File
		fileInfo os.FileInfo
		currHash []byte
		err error
	)

	// Stat the file
	fileInfo, err = os.Stat(filename)
	if err != nil {
		// If the stat didn't fail because the file doesn't exist, error
		if ! os.IsNotExist(err) {
			return nil, nil, err
		}

		// Create a new file
		log.Println("Creating a new file")
		file, err = os.Create(filename)
		if err != nil {
			return nil, nil, err
		}

		return file, make([]byte, 0), nil
	}

	// Open the file
	file, err = os.OpenFile(filename, os.O_RDWR, fileInfo.Mode())
	if err != nil {
		return nil, nil, err
	}

	// Hash the file
	currHash, err = Hash(file)
	if err != nil {
		return file, nil, err
	}

	return file, currHash, nil
}

func Download(f *os.File, url string) error {
	var (
		resp *http.Response
		written int64
		err error
	)

	// Reset the file
	err = resetFile(f)
	if err != nil {
		return err
	}

	// Request the page
	resp, err = http.Get(url)
	if err != nil {
		return err
	}

	// Write the response body to the file
	written, err = io.Copy(f, resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	log.Println("Downloaded", written, "bytes")

	// Trim off any excess bytes
	err = f.Truncate(written)

	return err
}

func Hash(file *os.File) ([]byte, error) {
	var (
		hash hash.Hash
		written int64
		err error
	)

	// Reset the file
	err = resetFile(file)
	if err != nil {
		return nil, err
	}

	// Hash the file
	hash = md5.New()
	written, err = io.Copy(hash, file)
	if err != nil {
		return nil, err
	}

	log.Println("Hashed", written, "bytes")
	return hash.Sum(nil), nil
}

func resetFile(file *os.File) error {
	_, err := file.Seek(0, os.SEEK_SET)
	return err
}

func SendNotification(smtpConf SmtpConfig, emailConf EmailConfig, url string) error {
	type Email struct {
		To, From, Subject, Url string
	}

	var (
		email Email
		templ *template.Template
		addr string
		auth smtp.Auth
		body *bytes.Buffer
		conn net.Conn
		c *smtp.Client
		w io.WriteCloser
		err error
	)

	addr = smtpConf.Host + ":" + strconv.FormatUint(uint64(smtpConf.Port), 10)
	auth = smtp.PlainAuth("", smtpConf.Username, smtpConf.Password, addr)

	// Load the body template
	templ, err = template.ParseFiles(emailConf.BodyTmpl...)
	if err != nil {
		log.Println("Failed to parse the template")
		return err
	}

	// Connect to the smtp server
	conn, err = net.Dial("tcp", addr)
	if err != nil {
		log.Println("Error connecting")
		return err
	}

	// Create an smtp client
	c, err = smtp.NewClient(conn, addr)
	if err != nil {
		log.Println("Error creating the client")
		return err
	}

	// Start a TLS encryption session
	err = c.StartTLS(nil)
	if err != nil {
		log.Println("Error starting the TLS session")
		c.Quit()
		return err
	}

	// Authenticate with the smtp server
	err = c.Auth(auth)
	if err != nil {
		log.Println("Error authenticating")
		c.Quit()
		return err
	}

	// Send an email to each recipient
	email = Email { "", emailConf.From, emailConf.Subject, url }
	body = bytes.NewBuffer(make([]byte, 0, 500))
	for i := 0 ; i < len(emailConf.To) ; i++ {
		// Set the recipient
		email.To = emailConf.To[i]

		// Explode the body template
		err = templ.Execute(body, email)
		if err != nil {
			log.Println("Failed to expand the template")
			c.Quit()
			return err
		}

		log.Printf("emailing: %+v\n", email)
		log.Printf("body: %s\n", string(body.Bytes()))

		// Set addresses
		c.Mail(email.From)
		c.Rcpt(email.To)

		// Start the body of the email
		w, err = c.Data()
		if err != nil {
			log.Println("Error establishing the data connection")
			c.Quit()
			return err
		}

		// Write the body
		_, err = body.WriteTo(w)
		if err != nil {
			log.Println("Error writing data")
			w.Close()
			c.Quit()
			return err
		}
		w.Close()
		body.Reset()
	}

	c.Quit()

	return nil
}

func hasBuyButton(f *os.File) (bool, error) {
	// Read in the file's contents
	contents, err := ioutil.ReadFile(f.Name())
	if err != nil {
		return false, err
	}
	// Compile the regular expression
	regex, err := regexp.Compile("alt=['\"]Buy Tickets['\"]")
	if err != nil {
		return false, err
	}
	// Search for the buy button
	found := regex.Find(contents)
	return found != nil, nil
}

type SmtpConfig struct {
	Host string
	Port uint16
	Username string
	Password string
}

type EmailConfig struct {
	Subject string
	From string
	To []string
	BodyTmpl []string
}

