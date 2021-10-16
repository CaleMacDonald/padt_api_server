package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"github.com/google/uuid"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"time"
)

type key int

const (
	requestIDKey key = 0
)

var (
	listenAddr string
	healthy    int32
	debug      bool
	file       string
)

func main() {
	flag.StringVar(&listenAddr, "listen-addr", ":5000", "server listen address")
	flag.StringVar(&file, "file", "padt_response_file.xml", "The file to read")
	flag.BoolVar(&debug, "debug", false, "Include logging of request details")
	flag.Parse()

	logger := log.New(os.Stdout, "http: ", log.LstdFlags)
	logger.Printf("Serving file %s\n", file)
	logger.Println("Server is starting...")

	router := http.NewServeMux()
	router.Handle("/", index())
	router.Handle("/healthz", healthz())
	router.Handle("/padt", sendPadtResponse())

	nextRequestID := func() string {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}

	server := &http.Server{
		Addr:         listenAddr,
		Handler:      tracing(nextRequestID)(logging(logger)(router)),
		ErrorLog:     logger,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	done := make(chan bool)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	go func() {
		<-quit
		logger.Println("Server is shutting down...")
		atomic.StoreInt32(&healthy, 0)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		server.SetKeepAlivesEnabled(false)
		if err := server.Shutdown(ctx); err != nil {
			logger.Fatalf("Could not gracefully shutdown the server: %v\n", err)
		}
		close(done)
	}()

	logger.Println("Server is ready to handle requests at", listenAddr)
	atomic.StoreInt32(&healthy, 1)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("Could not listen on %s: %v\n", listenAddr, err)
	}

	<-done
	logger.Println("Server stopped")
}

func index() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "use /padt as the URL to POST to")
	})
}

func healthz() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt32(&healthy) == 1 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
}

func sendPadtResponse() http.Handler {
	filePath := file
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, err := ioutil.ReadFile(file)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Unable to read %s\nServing default response\n", filePath)
			file = []byte(getDefaultResponse())
		}

		if debug {
			fmt.Fprintln(os.Stdout, "--------------")
			for k, v := range r.Header {
				fmt.Fprintf(os.Stdout, "%q: %q\n", k, v)
			}
			body, _ := ioutil.ReadAll(r.Body)
			fmt.Fprintf(os.Stdout, string(body))
			fmt.Fprintln(os.Stdout, "--------------")
		}

		fileContent := string(file)

		partyId, err := uuid.NewRandom()
		if err == nil {
			fileContent = strings.ReplaceAll(fileContent, "${PartyID}", partyId.String())
		}

		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(fileContent))
	})
}

func logging(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				requestID, ok := r.Context().Value(requestIDKey).(string)
				if !ok {
					requestID = "unknown"
				}
				logger.Println(requestID, r.Method, r.URL.Path, r.RemoteAddr, r.UserAgent())

				buf := new(bytes.Buffer)
				bytesRead, err := buf.ReadFrom(r.Body)
				if err == nil && bytesRead > 0 {
					logger.Println("-----------------")
					logger.Println(buf.String())
					logger.Println("-----------------")
				}

			}()
			next.ServeHTTP(w, r)
		})
	}
}

func tracing(nextRequestID func() string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get("X-Request-Id")
			if requestID == "" {
				requestID = nextRequestID()
			}
			ctx := context.WithValue(r.Context(), requestIDKey, requestID)
			w.Header().Set("X-Request-Id", requestID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func getDefaultResponse() string {
	return "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>\n<ManagePartyResponse xmlns=\"urn:inputoutput.wm.ms.com\" xmlns:ns6=\"urn:group.accounts.wm.ms.com\" xmlns:ns5=\"urn:user.wm.ms.com\" xmlns:ns8=\"urn:edd.wm.ms.com\" xmlns:ns7=\"urn:accounts.wm.ms.com\" xmlns:ns9=\"urn:partyext.wm.ms.com\" xmlns:ns11=\"urn:products.wm.ms.com\" xmlns:ns10=\"urn:operation.wm.ms.com\" xmlns:ns2=\"urn:basetypes.wm.ms.com\" xmlns:ns4=\"urn:Common.wm.ms.com\" xmlns:ns3=\"urn:party.wm.ms.com\">\n    <Parties>\n        <Party>\n            <ns9:Party>\n                <ns3:PartyId>\n                    <ns2:ID>3209601986633|9WW</ns2:ID>\n                    <ns2:IDType>\n                        <ns2:Code>CES</ns2:Code>\n                        <ns2:Value>CES</ns2:Value>\n                    </ns2:IDType>\n                </ns3:PartyId>\n                <ns3:PartyId>\n                    <ns2:ID>0607076334</ns2:ID>\n                    <ns2:IDType>\n                        <ns2:Code>PMH</ns2:Code>\n                    </ns2:IDType>\n                </ns3:PartyId>\n                <ns3:PartyType>\n                    <ns2:Code>I</ns2:Code>\n                    <ns2:Value>Individual</ns2:Value>\n                </ns3:PartyType>\n                <ns3:RelationshipType>\n                    <ns2:Code>P</ns2:Code>\n                    <ns2:Value>Prospect</ns2:Value>\n                </ns3:RelationshipType>\n                <ns3:NameInfo>\n                    <ns2:FirstName>LALA</ns2:FirstName>\n                    <ns2:LastName>SMITH</ns2:LastName>\n                    <ns2:IsInvalidName>false</ns2:IsInvalidName>\n                </ns3:NameInfo>\n                <ns3:Tax>\n                    <ns2:TaxType>\n                        <ns2:Code>S</ns2:Code>\n                        <ns2:Value>SSN</ns2:Value>\n                    </ns2:TaxType>\n                    <ns2:TaxId>765712215</ns2:TaxId>\n                    <ns2:IsInvalidTaxId>false</ns2:IsInvalidTaxId>\n                </ns3:Tax>\n                <ns3:Individual>\n                    <ns2:DateOfBirth>1983-04-22</ns2:DateOfBirth>\n                    <ns2:IsInvalidDateOfBirth>false</ns2:IsInvalidDateOfBirth>\n                </ns3:Individual>\n                <ns3:Contact>\n                    <ns2:ContactAddress>\n                        <ns2:Address TxType=\"C\">\n                            <ns2:AddressRowid>4140026</ns2:AddressRowid>\n                            <ns2:AddressRelRowid>1LGL</ns2:AddressRelRowid>\n                            <ns2:AddressId>A05001855978</ns2:AddressId>\n                            <ns2:AddressType>\n                                <ns2:Code>LGL</ns2:Code>\n                                <ns2:Value>Client Legal Address</ns2:Value>\n                            </ns2:AddressType>\n                            <ns2:StreetAddress1>Mysore Rd, Opp Bhel, Nayandanahalli</ns2:StreetAddress1>\n                            <ns2:City>Bangalore</ns2:City>\n                            <ns2:PostalCode>560039</ns2:PostalCode>\n                            <ns2:Country>\n                                <ns2:Code>IND</ns2:Code>\n                            </ns2:Country>\n                            <ns2:ForeignAddress>\n                                <ns2:Code>FRN</ns2:Code>\n                            </ns2:ForeignAddress>\n                            <ns2:IsValid>true</ns2:IsValid>\n                        </ns2:Address>\n                        <ns2:VanityAddress>\n                            <ns2:StreetAddress1>Mysore Rd, Opp Bhel, Nayandanahalli</ns2:StreetAddress1>\n                            <ns2:City>Bangalore</ns2:City>\n                            <ns2:Postal>560039</ns2:Postal>\n                            <ns2:Country>\n                                <ns2:Code>IND</ns2:Code>\n                                <ns2:Value>INDIA</ns2:Value>\n                            </ns2:Country>\n                        </ns2:VanityAddress>\n                    </ns2:ContactAddress>\n                    <ns2:Telephone TxType=\"C\">\n                        <ns2:TelephoneRowid>1740042</ns2:TelephoneRowid>\n                        <ns2:PhoneRelRowid>1CELL</ns2:PhoneRelRowid>\n                        <ns2:TelephoneId>T05000622214</ns2:TelephoneId>\n                        <ns2:PhoneNumber>+91-9872710627</ns2:PhoneNumber>\n                        <ns2:PhoneType>\n                            <ns2:Code>CELL</ns2:Code>\n                            <ns2:Value>Mobile Phone</ns2:Value>\n                        </ns2:PhoneType>\n                        <ns2:AuditData/>\n                    </ns2:Telephone>\n                    <ns2:ElectronicAddress TxType=\"C\">\n                        <ns2:ElectronicAddressRowid>1000071</ns2:ElectronicAddressRowid>\n                        <ns2:ElectronicRelRowid>1HOMEML</ns2:ElectronicRelRowid>\n                        <ns2:ElectronicAddressId>E05000384759</ns2:ElectronicAddressId>\n                        <ns2:ElectronicAddress>lala.smith@sso.com</ns2:ElectronicAddress>\n                        <ns2:ElectronicAddressMethod>\n                            <ns2:Code>EMAIL</ns2:Code>\n                            <ns2:Value>Email</ns2:Value>\n                        </ns2:ElectronicAddressMethod>\n                        <ns2:ElectronicAddressType>\n                            <ns2:Code>HOMEML</ns2:Code>\n                            <ns2:Value>Home Email</ns2:Value>\n                        </ns2:ElectronicAddressType>\n                    </ns2:ElectronicAddress>\n                </ns3:Contact>\n                <ns3:IsTestParty>false</ns3:IsTestParty>\n                <ns3:AdditionalInfo>\n                    <ns2:Code>EDBSync</ns2:Code>\n                    <ns2:Value>N</ns2:Value>\n                </ns3:AdditionalInfo>\n                <ns3:AdditionalInfo>\n                    <ns2:Code>MATCH_STA</ns2:Code>\n                    <ns2:Value>NOT-MATCHED</ns2:Value>\n                </ns3:AdditionalInfo>\n                <ns3:AdditionalInfo>\n                    <ns2:Code>CoreDataUpdate</ns2:Code>\n                    <ns2:Value>Yes</ns2:Value>\n                </ns3:AdditionalInfo>\n                <ns3:AdditionalInfo>\n                    <ns2:Code>Created</ns2:Code>\n                    <ns2:Value>Prospect</ns2:Value>\n                </ns3:AdditionalInfo>\n                <ns3:AdditionalInfo>\n                    <ns2:Code>isCodeTranslationRequired</ns2:Code>\n                    <ns2:Value>true</ns2:Value>\n                </ns3:AdditionalInfo>\n                <ns3:AdditionalInfo>\n                    <ns2:Code>APPTYPE</ns2:Code>\n                    <ns2:Value>00</ns2:Value>\n                </ns3:AdditionalInfo>\n                <ns3:ChannelType>\n                    <ns2:Code>PRODUCT_CODE</ns2:Code>\n                    <ns2:Value>SDB</ns2:Value>\n                </ns3:ChannelType>\n                <ns3:ChannelType>\n                    <ns2:Code>MSA</ns2:Code>\n                    <ns2:Value>Y</ns2:Value>\n                </ns3:ChannelType>\n                <ns3:PlanParticipation>\n                    <ns2:Identifier>\n                        <ns2:ID>9WW</ns2:ID>\n                        <ns2:IDType>CORP_ID</ns2:IDType>\n                    </ns2:Identifier>\n                    <ns2:FA/>\n                    <ns2:SourceApplicationCode>\n                        <ns2:Code>CS-SN</ns2:Code>\n                        <ns2:Value>Corporate Solution Solium Native</ns2:Value>\n                    </ns2:SourceApplicationCode>\n                </ns3:PlanParticipation>\n            </ns9:Party>\n        </Party>\n    </Parties>\n    <TransactionInfo>\n        <ns10:EventCorrelationId>4f04baaf-a355-4d4a-945e-54997f5c8594</ns10:EventCorrelationId>\n        <ns10:EventTimeStamp>2021-09-21T08:28:40.351-04:00</ns10:EventTimeStamp>\n        <ns10:EventSource>SHAREWORKS_IDP</ns10:EventSource>\n        <ns10:EventName>PROSPECT.ADD</ns10:EventName>\n        <ns10:TransactionActionType>MANAGE_PROSPECT Call from SHAREWORKS</ns10:TransactionActionType>\n        <ns10:TransactionSource>CES</ns10:TransactionSource>\n        <ns10:TransactionTimeStamp>2021-09-21T08:28:40.351-04:00</ns10:TransactionTimeStamp>\n        <ns10:TransactionUser>SUM_Shareworks</ns10:TransactionUser>\n        <ns10:TransactionProgram>SHAREWORKS_IDP</ns10:TransactionProgram>\n        <ns10:UseCaseNumber>IDP</ns10:UseCaseNumber>\n    </TransactionInfo>\n    <StatusInfo>\n        <ns10:Code>000</ns10:Code>\n        <ns10:TechnicalDescription>CreateParty Success</ns10:TechnicalDescription>\n        <ns10:BusinessDescription>SUCCESS</ns10:BusinessDescription>\n        <ns10:Retryable>false</ns10:Retryable>\n    </StatusInfo>\n</ManagePartyResponse>\n"
}
