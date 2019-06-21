package soap

import (
	"context"
	"encoding/xml"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

type request struct {
	XMLName xml.Name `xml:"test:call Request"`
	Attr1   string   `xml:"attr1,omitempty"`
	Attr2   string   `xml:"attr2,omitempty"`
}

type response struct {
	XMLName xml.Name `xml:"test:call Response"`
	Attr3   string   `xml:"attr3,omitempty"`
}

func Test_Marshal(t *testing.T) {
	t.Parallel()
	env := Envelope{Body: Body{Content: request{Attr1: "value1", Attr2: "value2"}}}
	b, err := xml.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	want := `<Envelope xmlns="http://schemas.xmlsoap.org/soap/envelope/"><Body xmlns="http://schemas.xmlsoap.org/soap/envelope/"><Request xmlns="test:call"><attr1>value1</attr1><attr2>value2</attr2></Request></Body></Envelope>`
	if got := string(b); got != want {
		t.Fatalf("got: %s, want: %s", got, want)
	}
}

func TestClient_Call(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if want := "POST"; r.Method != want {
			t.Fatalf("got: %s, want: %s", r.Method, want)
		}

		ha := r.Header.Get("SOAPAction")
		if want := "soap.action"; ha != want {
			t.Fatalf("got: %s, want: %s", ha, want)
		}

		body, berr := ioutil.ReadAll(r.Body)
		if berr != nil {
			t.Fatal(berr)
		}

		var req request
		soapreq := Envelope{Body: Body{Content: &req}}
		if err := xml.Unmarshal(body, &soapreq); err != nil {
			t.Fatal(err)
		}

		if want := "value1"; req.Attr1 != want {
			t.Fatalf("got: %s, want: %s", req.Attr1, want)
		}

		if want := "value2"; req.Attr2 != want {
			t.Fatalf("got: %s, want: %s", req.Attr2, want)
		}

		soapresp := Envelope{Body: Body{Content: response{Attr3: "value3"}}}
		enc := xml.NewEncoder(w)
		if err := enc.Encode(soapresp); err != nil {
			t.Fatal(err)
		}

		if err := enc.Flush(); err != nil {
			t.Fatal(err)
		}
	}))
	defer srv.Close()

	var r response
	if err := NewClient(srv.URL, Config{}).Call(context.Background(), "soap.action", request{
		Attr1: "value1",
		Attr2: "value2",
	}, &r); err != nil {
		t.Fatal(err)
	}

	if want := "value3"; r.Attr3 != want {
		t.Fatalf("got: %s, want: %s", r.Attr3, want)
	}
}

func TestClient_EmptyAction(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := xml.Marshal(Envelope{Body: Body{}})
		w.Write(b)
	}))
	defer srv.Close()

	if err := NewClient(srv.URL, Config{}).Call(context.Background(), "", request{}, nil); err != nil {
		t.Fatal(err)
	}
}

func Test_Fault(t *testing.T) {
	t.Parallel()
	for i, v := range []struct {
		fault *Fault
		want  string
	}{
		{want: "soap: <nil>"},
		{fault: &Fault{Code: "code", Text: "text", HTTPStatus: 500}, want: "soap: code: text 500"},
		{fault: &Fault{Text: "text", Detail: "detail"}, want: "soap: text (detail)"},
		{fault: &Fault{Code: "code", Text: "text", Detail: "detail"}, want: "soap: code: text (detail)"},
	} {
		if v.fault.Error() != v.want {
			t.Errorf("#%d got: %s, want: %s", i, v.fault.Error(), v.want)
		}
	}
}

func TestClient_Fault(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		soapresp := Envelope{Body: Body{Fault: &Fault{Text: "      fault text"}}}

		w.WriteHeader(500)
		enc := xml.NewEncoder(w)
		if err := enc.Encode(soapresp); err != nil {
			t.Fatal(err)
		}

		if err := enc.Flush(); err != nil {
			t.Fatal(err)
		}
	}))
	defer srv.Close()

	want := "soap: fault text 500"
	if err := NewClient(srv.URL, Config{}).Call(context.Background(), "", request{}, nil); err.Error() != want {
		t.Fatalf("got: %s, want: %s", err, want)
	}
}

func TestClient_EmptyBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	if err := NewClient(srv.URL, Config{}).Call(context.Background(), "", request{}, nil); err != errBody {
		t.Fatalf("got: %s, want: %s", err, errBody)
	}
}

func TestClient_BasicAuth(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			t.Fatal("no basic auth")
		}

		if want := "user"; username != want {
			t.Fatalf("got: %s, want: %s", username, want)
		}
		if want := "pass"; password != want {
			t.Fatalf("got: %s, want: %s", password, want)
		}
	}))
	defer srv.Close()

	NewClient(srv.URL, Config{BasicAuth: &BasicAuth{Username: "user", Password: "pass"}}).Call(context.Background(), "", request{}, nil)
}

/*func TestClient_AddHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "gopher"
		if got := r.Header.Get("X-Custom"); got != want {
			t.Fatalf("got: %s, want: %s", got, want)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, Config{})
	client.AddHeader(Header{})
	client.Call("", request{}, nil)
}*/
