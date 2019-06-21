package soap

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
)

var (
	errUnauthorized = fmt.Errorf("soap: unauthorized")
	errBody         = fmt.Errorf("soap: body response is empty")
)

// Envelope implements soap envelope.
type Envelope struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
	Header  *Header
	Body    Body
}

// Header implements soap headers.
type Header struct {
	XMLName xml.Name      `xml:"http://schemas.xmlsoap.org/soap/envelope/ Header"`
	Items   []interface{} `xml:",omitempty"`
}

// Body implements soap body.
type Body struct {
	XMLName xml.Name    `xml:"http://schemas.xmlsoap.org/soap/envelope/ Body"`
	Fault   *Fault      `xml:",omitempty"`
	Content interface{} `xml:",omitempty"`
}

// UnmarshalXML implements xml.Unmarshaler interface.
func (b *Body) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	if b.Content == nil {
		return xml.UnmarshalError("content must be a pointer to a struct")
	}

	var (
		token    xml.Token
		err      error
		consumed bool
	)

Loop:
	for {
		if token, err = d.Token(); err != nil {
			return err
		}

		if token == nil {
			break
		}

		switch se := token.(type) {
		case xml.StartElement:
			if consumed {
				return xml.UnmarshalError("found multiple elements inside SOAP body; not wrapped-document/literal WS-I compliant")
			} else if se.Name.Space == "http://schemas.xmlsoap.org/soap/envelope/" && se.Name.Local == "Fault" {
				b.Fault = &Fault{}
				b.Content = nil

				err = d.DecodeElement(b.Fault, &se)
				if err != nil {
					return err
				}

				consumed = true
			} else {
				if err = d.DecodeElement(b.Content, &se); err != nil {
					return err
				}

				consumed = true
			}
		case xml.EndElement:
			break Loop
		}
	}

	return nil
}

type trimSpace string

func (ts *trimSpace) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var v string
	if err := d.DecodeElement(&v, &start); err != nil {
		return err
	}

	*ts = trimSpace(strings.TrimSpace(v))
	return nil
}

func (ts trimSpace) String() string {
	return string(ts)
}

// Fault implements soap fault.
type Fault struct {
	XMLName    xml.Name  `xml:"http://schemas.xmlsoap.org/soap/envelope/ Fault"`
	Code       trimSpace `xml:"faultcode,omitempty"`
	Text       trimSpace `xml:"faultstring,omitempty"`
	Actor      trimSpace `xml:"faultactor,omitempty"`
	Detail     trimSpace `xml:"detail,omitempty"`
	HTTPStatus int       `xml:"-"`
}

func (f *Fault) Error() string {
	if f == nil {
		return "soap: <nil>"
	}

	err := f.Text.String()
	if f.Code != "" {
		err = fmt.Sprintf("%s: %s", f.Code, err)
	}

	if f.Detail != "" {
		err += fmt.Sprintf(" (%s)", f.Detail.String())
	}

	if f.HTTPStatus != 0 {
		err += fmt.Sprintf(" %d", f.HTTPStatus)
	}

	return "soap: " + err
}

// Config implements config of the soap client.
type Config struct {
	BasicAuth           *BasicAuth
	TLS                 *tls.Config
	MaxIdleConnsPerHost int
}

// Client implements soap client.
type Client struct {
	url        string
	auth       *BasicAuth
	headers    []interface{}
	httpClient *http.Client
}

// NewClient creates soap client.
func NewClient(url string, c Config) *Client {
	return &Client{
		url:  url,
		auth: c.BasicAuth,
		httpClient: &http.Client{Transport: &http.Transport{
			TLSClientConfig: c.TLS,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, addr)
			},
			MaxIdleConnsPerHost: c.MaxIdleConnsPerHost,
		}},
	}
}

// BasicAuth implements work with basic authorization.
type BasicAuth struct {
	Username string
	Password string
}

// AddHeader adds header.
func (s *Client) AddHeader(header interface{}) {
	s.headers = append(s.headers, header)
}

// Call sends soap request.
func (s *Client) Call(ctx context.Context, soapAction string, request, response interface{}) error {
	// action may be empty
	if response == nil {
		response = new(interface{})
	}

	var envelope Envelope
	if s.headers != nil && len(s.headers) > 0 {
		soapHeader := &Header{Items: make([]interface{}, len(s.headers))}
		copy(soapHeader.Items, s.headers)
		envelope.Header = soapHeader
	}

	envelope.Body.Content = request
	buffer := new(bytes.Buffer)

	encoder := xml.NewEncoder(buffer)
	//encoder.Indent("  ", "    ")
	if err := encoder.Encode(envelope); err != nil {
		return fmt.Errorf("soap: %s", err)
	}
	if err := encoder.Flush(); err != nil {
		return fmt.Errorf("soap: %s", err)
	}

	req, err := http.NewRequest("POST", s.url, buffer)
	if err != nil {
		return fmt.Errorf("soap: %s", err)
	}
	if s.auth != nil {
		req.SetBasicAuth(s.auth.Username, s.auth.Password)
	}
	req.Header.Add("Content-Type", "text/xml; charset=\"utf-8\"")
	req.Header.Add("SOAPAction", soapAction)
	req.Close = true

	resp, err := s.httpClient.Do(req.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("soap: %s", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("soap: %s", err)
	}

	// body must not be empty
	if len(body) == 0 {
		return errBody
	}

	if resp.StatusCode == 401 {
		return errUnauthorized
	}

	respEnvelope := &Envelope{Body: Body{Content: response}}
	err = xml.Unmarshal(body, respEnvelope)
	if err != nil {
		return fmt.Errorf("soap: %s (%d)", resp.Status, resp.StatusCode)
	}

	// check fault
	if respEnvelope.Body.Fault != nil {
		respEnvelope.Body.Fault.HTTPStatus = resp.StatusCode
		return respEnvelope.Body.Fault
	}
	return nil
}
