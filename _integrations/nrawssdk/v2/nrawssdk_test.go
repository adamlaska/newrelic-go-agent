package nrawssdk

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/endpoints"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	newrelic "github.com/newrelic/go-agent"
	"github.com/newrelic/go-agent/internal"
)

func testApp(t *testing.T) newrelic.Application {
	cfg := newrelic.NewConfig("appname", "0123456789012345678901234567890123456789")
	cfg.Enabled = false
	cfg.DistributedTracer.Enabled = true

	app, err := newrelic.NewApplication(cfg)
	if nil != err {
		t.Fatal(err)
	}

	replyfn := func(reply *internal.ConnectReply) {
		reply.AdaptiveSampler = internal.SampleEverything{}
	}

	internal.HarvestTesting(app, replyfn)
	return app
}

type fakeTransport struct{}

func (t fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Body:       ioutil.NopCloser(bytes.NewReader([]byte(""))),
		Header: http.Header{
			"X-Amzn-Requestid": []string{requestID},
		},
	}, nil
}

type fakeCreds struct{}

func (c fakeCreds) Retrieve() (aws.Credentials, error) {
	return aws.Credentials{}, nil
}

func newConfig(instrument bool) aws.Config {
	cfg, _ := external.LoadDefaultAWSConfig()
	cfg.Credentials = fakeCreds{}
	cfg.Region = endpoints.UsWest2RegionID
	cfg.HTTPClient = &http.Client{
		Transport: &fakeTransport{},
	}

	if instrument {
		InstrumentHandlers(&cfg.Handlers)
	}
	return cfg
}

const (
	requestID = "testing request id"
	txnName   = "aws-txn"
)

var (
	genericSpan = internal.WantEvent{
		Intrinsics: map[string]interface{}{
			"name":          "OtherTransaction/Go/" + txnName,
			"sampled":       true,
			"category":      "generic",
			"priority":      internal.MatchAnything,
			"guid":          internal.MatchAnything,
			"transactionId": internal.MatchAnything,
			"nr.entryPoint": true,
			"traceId":       internal.MatchAnything,
		},
		UserAttributes:  map[string]interface{}{},
		AgentAttributes: map[string]interface{}{},
	}
	externalSpan = internal.WantEvent{
		Intrinsics: map[string]interface{}{
			"name":          "External/lambda.us-west-2.amazonaws.com/http/POST",
			"sampled":       true,
			"category":      "http",
			"priority":      internal.MatchAnything,
			"guid":          internal.MatchAnything,
			"transactionId": internal.MatchAnything,
			"traceId":       internal.MatchAnything,
			"parentId":      internal.MatchAnything,
			"component":     "http",
			"span.kind":     "client",
		},
		UserAttributes: map[string]interface{}{},
		AgentAttributes: map[string]interface{}{
			"aws.operation": "Invoke",
			"aws.region":    "us-west-2",
			"aws.requestId": requestID,
			"http.method":   "POST",
			"http.url":      "https://lambda.us-west-2.amazonaws.com/2015-03-31/functions/non-existent-function/invocations",
		},
	}
	externalSpanNoRequestID = internal.WantEvent{
		Intrinsics: map[string]interface{}{
			"name":          "External/lambda.us-west-2.amazonaws.com/http/POST",
			"sampled":       true,
			"category":      "http",
			"priority":      internal.MatchAnything,
			"guid":          internal.MatchAnything,
			"transactionId": internal.MatchAnything,
			"traceId":       internal.MatchAnything,
			"parentId":      internal.MatchAnything,
			"component":     "http",
			"span.kind":     "client",
		},
		UserAttributes: map[string]interface{}{},
		AgentAttributes: map[string]interface{}{
			"aws.operation": "Invoke",
			"aws.region":    "us-west-2",
			"http.method":   "POST",
			"http.url":      "https://lambda.us-west-2.amazonaws.com/2015-03-31/functions/non-existent-function/invocations",
		},
	}
	datastoreSpan = internal.WantEvent{
		Intrinsics: map[string]interface{}{
			"name":          "Datastore/statement/DynamoDB/thebesttable/DescribeTable",
			"sampled":       true,
			"category":      "datastore",
			"priority":      internal.MatchAnything,
			"guid":          internal.MatchAnything,
			"transactionId": internal.MatchAnything,
			"traceId":       internal.MatchAnything,
			"parentId":      internal.MatchAnything,
			"component":     "DynamoDB",
			"span.kind":     "client",
		},
		UserAttributes: map[string]interface{}{},
		AgentAttributes: map[string]interface{}{
			"aws.operation": "DescribeTable",
			"aws.region":    "us-west-2",
			"aws.requestId": requestID,
			"db.collection": "thebesttable",
			"db.statement":  "'DescribeTable' on 'thebesttable' using 'DynamoDB'",
			"peer.address":  "dynamodb.us-west-2.amazonaws.com:unknown",
			"peer.hostname": "dynamodb.us-west-2.amazonaws.com",
		},
	}

	txnMetrics = []internal.WantMetric{
		{Name: "DurationByCaller/Unknown/Unknown/Unknown/Unknown/all", Scope: "", Forced: false, Data: nil},
		{Name: "DurationByCaller/Unknown/Unknown/Unknown/Unknown/allOther", Scope: "", Forced: false, Data: nil},
		{Name: "OtherTransaction/Go/" + txnName, Scope: "", Forced: true, Data: nil},
		{Name: "OtherTransaction/all", Scope: "", Forced: true, Data: nil},
		{Name: "OtherTransactionTotalTime/Go/" + txnName, Scope: "", Forced: false, Data: nil},
		{Name: "OtherTransactionTotalTime", Scope: "", Forced: true, Data: nil},
	}
	externalMetrics = append(txnMetrics, []internal.WantMetric{
		{Name: "External/all", Scope: "", Forced: true, Data: nil},
		{Name: "External/allOther", Scope: "", Forced: true, Data: nil},
		{Name: "External/lambda.us-west-2.amazonaws.com/all", Scope: "", Forced: false, Data: nil},
		{Name: "External/lambda.us-west-2.amazonaws.com/http/POST", Scope: "OtherTransaction/Go/" + txnName, Forced: false, Data: nil},
	}...)
	datastoreMetrics = append(txnMetrics, []internal.WantMetric{
		{Name: "Datastore/DynamoDB/all", Scope: "", Forced: true, Data: nil},
		{Name: "Datastore/DynamoDB/allOther", Scope: "", Forced: true, Data: nil},
		{Name: "Datastore/all", Scope: "", Forced: true, Data: nil},
		{Name: "Datastore/allOther", Scope: "", Forced: true, Data: nil},
		{Name: "Datastore/instance/DynamoDB/dynamodb.us-west-2.amazonaws.com/unknown", Scope: "", Forced: false, Data: nil},
		{Name: "Datastore/operation/DynamoDB/DescribeTable", Scope: "", Forced: false, Data: nil},
		{Name: "Datastore/statement/DynamoDB/thebesttable/DescribeTable", Scope: "", Forced: false, Data: nil},
		{Name: "Datastore/statement/DynamoDB/thebesttable/DescribeTable", Scope: "OtherTransaction/Go/" + txnName, Forced: false, Data: nil},
	}...)
)

func TestInstrumentRequestExternal(t *testing.T) {
	app := testApp(t)
	txn := app.StartTransaction(txnName, nil, nil)

	client := lambda.New(newConfig(false))
	input := &lambda.InvokeInput{
		ClientContext:  aws.String("MyApp"),
		FunctionName:   aws.String("non-existent-function"),
		InvocationType: lambda.InvocationTypeEvent,
		LogType:        lambda.LogTypeTail,
		Payload:        []byte("{}"),
	}
	req := client.InvokeRequest(input)
	InstrumentHandlers(&req.Handlers)
	ctx := newrelic.NewContext(req.Context(), txn)

	_, err := req.Send(ctx)
	if nil != err {
		t.Error(err)
	}

	txn.End()

	app.(internal.Expect).ExpectMetrics(t, externalMetrics)
	app.(internal.Expect).ExpectSpanEvents(t, []internal.WantEvent{
		genericSpan, externalSpan})
}

func TestInstrumentRequestDatastore(t *testing.T) {
	app := testApp(t)
	txn := app.StartTransaction(txnName, nil, nil)

	client := dynamodb.New(newConfig(false))
	input := &dynamodb.DescribeTableInput{
		TableName: aws.String("thebesttable"),
	}

	req := client.DescribeTableRequest(input)
	InstrumentHandlers(&req.Handlers)
	ctx := newrelic.NewContext(req.Context(), txn)

	_, err := req.Send(ctx)
	if nil != err {
		t.Error(err)
	}

	txn.End()

	app.(internal.Expect).ExpectMetrics(t, datastoreMetrics)
	app.(internal.Expect).ExpectSpanEvents(t, []internal.WantEvent{
		genericSpan, datastoreSpan})
}

func TestInstrumentRequestExternalNoTxn(t *testing.T) {
	client := lambda.New(newConfig(false))
	input := &lambda.InvokeInput{
		ClientContext:  aws.String("MyApp"),
		FunctionName:   aws.String("non-existent-function"),
		InvocationType: lambda.InvocationTypeEvent,
		LogType:        lambda.LogTypeTail,
		Payload:        []byte("{}"),
	}

	req := client.InvokeRequest(input)
	InstrumentHandlers(&req.Handlers)
	ctx := req.Context()

	_, err := req.Send(ctx)
	if nil != err {
		t.Error(err)
	}
}

func TestInstrumentRequestDatastoreNoTxn(t *testing.T) {
	client := dynamodb.New(newConfig(false))
	input := &dynamodb.DescribeTableInput{
		TableName: aws.String("thebesttable"),
	}

	req := client.DescribeTableRequest(input)
	InstrumentHandlers(&req.Handlers)
	ctx := req.Context()

	_, err := req.Send(ctx)
	if nil != err {
		t.Error(err)
	}
}

func TestInstrumentConfigExternal(t *testing.T) {
	app := testApp(t)
	txn := app.StartTransaction(txnName, nil, nil)

	client := lambda.New(newConfig(true))

	input := &lambda.InvokeInput{
		ClientContext:  aws.String("MyApp"),
		FunctionName:   aws.String("non-existent-function"),
		InvocationType: lambda.InvocationTypeEvent,
		LogType:        lambda.LogTypeTail,
		Payload:        []byte("{}"),
	}

	req := client.InvokeRequest(input)
	ctx := newrelic.NewContext(req.Context(), txn)

	_, err := req.Send(ctx)
	if nil != err {
		t.Error(err)
	}

	txn.End()

	app.(internal.Expect).ExpectMetrics(t, externalMetrics)
	app.(internal.Expect).ExpectSpanEvents(t, []internal.WantEvent{
		genericSpan, externalSpan})
}

func TestInstrumentConfigDatastore(t *testing.T) {
	app := testApp(t)
	txn := app.StartTransaction(txnName, nil, nil)

	client := dynamodb.New(newConfig(true))

	input := &dynamodb.DescribeTableInput{
		TableName: aws.String("thebesttable"),
	}

	req := client.DescribeTableRequest(input)
	ctx := newrelic.NewContext(req.Context(), txn)

	_, err := req.Send(ctx)
	if nil != err {
		t.Error(err)
	}

	txn.End()

	app.(internal.Expect).ExpectMetrics(t, datastoreMetrics)
	app.(internal.Expect).ExpectSpanEvents(t, []internal.WantEvent{
		genericSpan, datastoreSpan})
}

func TestInstrumentConfigExternalNoTxn(t *testing.T) {
	client := lambda.New(newConfig(true))

	input := &lambda.InvokeInput{
		ClientContext:  aws.String("MyApp"),
		FunctionName:   aws.String("non-existent-function"),
		InvocationType: lambda.InvocationTypeEvent,
		LogType:        lambda.LogTypeTail,
		Payload:        []byte("{}"),
	}

	req := client.InvokeRequest(input)
	ctx := req.Context()

	_, err := req.Send(ctx)
	if nil != err {
		t.Error(err)
	}
}

func TestInstrumentConfigDatastoreNoTxn(t *testing.T) {
	client := dynamodb.New(newConfig(true))

	input := &dynamodb.DescribeTableInput{
		TableName: aws.String("thebesttable"),
	}

	req := client.DescribeTableRequest(input)
	ctx := req.Context()

	_, err := req.Send(ctx)
	if nil != err {
		t.Error(err)
	}
}

func TestInstrumentConfigExternalTxnNotInCtx(t *testing.T) {
	app := testApp(t)
	txn := app.StartTransaction(txnName, nil, nil)

	client := lambda.New(newConfig(true))

	input := &lambda.InvokeInput{
		ClientContext:  aws.String("MyApp"),
		FunctionName:   aws.String("non-existent-function"),
		InvocationType: lambda.InvocationTypeEvent,
		LogType:        lambda.LogTypeTail,
		Payload:        []byte("{}"),
	}

	req := client.InvokeRequest(input)
	ctx := req.Context()

	_, err := req.Send(ctx)
	if nil != err {
		t.Error(err)
	}

	txn.End()

	app.(internal.Expect).ExpectMetrics(t, txnMetrics)
}

func TestInstrumentConfigDatastoreTxnNotInCtx(t *testing.T) {
	app := testApp(t)
	txn := app.StartTransaction(txnName, nil, nil)

	client := dynamodb.New(newConfig(true))

	input := &dynamodb.DescribeTableInput{
		TableName: aws.String("thebesttable"),
	}

	req := client.DescribeTableRequest(input)
	ctx := req.Context()

	_, err := req.Send(ctx)
	if nil != err {
		t.Error(err)
	}

	txn.End()

	app.(internal.Expect).ExpectMetrics(t, txnMetrics)
}

func TestDoublyInstrumented(t *testing.T) {
	hs := &aws.Handlers{}
	if found := hs.Send.Len(); 0 != found {
		t.Error("unexpected number of Send handlers found:", found)
	}

	InstrumentHandlers(hs)
	if found := hs.Send.Len(); 2 != found {
		t.Error("unexpected number of Send handlers found:", found)
	}

	InstrumentHandlers(hs)
	if found := hs.Send.Len(); 2 != found {
		t.Error("unexpected number of Send handlers found:", found)
	}
}

type firstFailingTransport struct {
	failing bool
}

func (t *firstFailingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if t.failing {
		t.failing = false
		return nil, errors.New("Oops this failed")
	}
	return &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Body:       ioutil.NopCloser(bytes.NewReader([]byte(""))),
		Header: http.Header{
			"X-Amzn-Requestid": []string{requestID},
		},
	}, nil
}

func TestRetrySend(t *testing.T) {
	app := testApp(t)
	txn := app.StartTransaction(txnName, nil, nil)

	cfg := newConfig(false)
	cfg.HTTPClient = &http.Client{
		Transport: &firstFailingTransport{failing: true},
	}

	client := lambda.New(cfg)
	input := &lambda.InvokeInput{
		ClientContext:  aws.String("MyApp"),
		FunctionName:   aws.String("non-existent-function"),
		InvocationType: lambda.InvocationTypeEvent,
		LogType:        lambda.LogTypeTail,
		Payload:        []byte("{}"),
	}
	req := client.InvokeRequest(input)
	InstrumentHandlers(&req.Handlers)
	ctx := newrelic.NewContext(req.Context(), txn)

	_, err := req.Send(ctx)
	if nil != err {
		t.Error(err)
	}

	txn.End()

	app.(internal.Expect).ExpectMetrics(t, []internal.WantMetric{
		{Name: "DurationByCaller/Unknown/Unknown/Unknown/Unknown/all", Scope: "", Forced: false, Data: nil},
		{Name: "DurationByCaller/Unknown/Unknown/Unknown/Unknown/allOther", Scope: "", Forced: false, Data: nil},
		{Name: "External/all", Scope: "", Forced: true, Data: []float64{2}},
		{Name: "External/allOther", Scope: "", Forced: true, Data: []float64{2}},
		{Name: "External/lambda.us-west-2.amazonaws.com/all", Scope: "", Forced: false, Data: []float64{2}},
		{Name: "External/lambda.us-west-2.amazonaws.com/http/POST", Scope: "OtherTransaction/Go/" + txnName, Forced: false, Data: []float64{2}},
		{Name: "OtherTransaction/Go/" + txnName, Scope: "", Forced: true, Data: nil},
		{Name: "OtherTransaction/all", Scope: "", Forced: true, Data: nil},
		{Name: "OtherTransactionTotalTime/Go/" + txnName, Scope: "", Forced: false, Data: nil},
		{Name: "OtherTransactionTotalTime", Scope: "", Forced: true, Data: nil},
	})
	app.(internal.Expect).ExpectSpanEvents(t, []internal.WantEvent{
		genericSpan, externalSpanNoRequestID, externalSpan})
}

func TestRequestSentTwice(t *testing.T) {
	app := testApp(t)
	txn := app.StartTransaction(txnName, nil, nil)

	client := lambda.New(newConfig(false))
	input := &lambda.InvokeInput{
		ClientContext:  aws.String("MyApp"),
		FunctionName:   aws.String("non-existent-function"),
		InvocationType: lambda.InvocationTypeEvent,
		LogType:        lambda.LogTypeTail,
		Payload:        []byte("{}"),
	}
	req := client.InvokeRequest(input)
	InstrumentHandlers(&req.Handlers)
	ctx := newrelic.NewContext(req.Context(), txn)

	_, firstErr := req.Send(ctx)
	if nil != firstErr {
		t.Error(firstErr)
	}

	_, secondErr := req.Send(ctx)
	if nil != secondErr {
		t.Error(secondErr)
	}

	txn.End()

	app.(internal.Expect).ExpectMetrics(t, []internal.WantMetric{
		{Name: "DurationByCaller/Unknown/Unknown/Unknown/Unknown/all", Scope: "", Forced: false, Data: nil},
		{Name: "DurationByCaller/Unknown/Unknown/Unknown/Unknown/allOther", Scope: "", Forced: false, Data: nil},
		{Name: "External/all", Scope: "", Forced: true, Data: []float64{2}},
		{Name: "External/allOther", Scope: "", Forced: true, Data: []float64{2}},
		{Name: "External/lambda.us-west-2.amazonaws.com/all", Scope: "", Forced: false, Data: []float64{2}},
		{Name: "External/lambda.us-west-2.amazonaws.com/http/POST", Scope: "OtherTransaction/Go/" + txnName, Forced: false, Data: []float64{2}},
		{Name: "OtherTransaction/Go/" + txnName, Scope: "", Forced: true, Data: nil},
		{Name: "OtherTransaction/all", Scope: "", Forced: true, Data: nil},
		{Name: "OtherTransactionTotalTime/Go/" + txnName, Scope: "", Forced: false, Data: nil},
		{Name: "OtherTransactionTotalTime", Scope: "", Forced: true, Data: nil},
	})
	app.(internal.Expect).ExpectSpanEvents(t, []internal.WantEvent{
		genericSpan, externalSpan, externalSpan})
}

type noRequestIDTransport struct{}

func (t *noRequestIDTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Body:       ioutil.NopCloser(bytes.NewReader([]byte(""))),
	}, nil
}

func TestNoRequestIDFound(t *testing.T) {
	app := testApp(t)
	txn := app.StartTransaction(txnName, nil, nil)

	cfg := newConfig(false)
	cfg.HTTPClient = &http.Client{
		Transport: &noRequestIDTransport{},
	}

	client := lambda.New(cfg)
	input := &lambda.InvokeInput{
		ClientContext:  aws.String("MyApp"),
		FunctionName:   aws.String("non-existent-function"),
		InvocationType: lambda.InvocationTypeEvent,
		LogType:        lambda.LogTypeTail,
		Payload:        []byte("{}"),
	}
	req := client.InvokeRequest(input)
	InstrumentHandlers(&req.Handlers)
	ctx := newrelic.NewContext(req.Context(), txn)

	_, err := req.Send(ctx)
	if nil != err {
		t.Error(err)
	}

	txn.End()

	app.(internal.Expect).ExpectMetrics(t, externalMetrics)
	app.(internal.Expect).ExpectSpanEvents(t, []internal.WantEvent{
		genericSpan, externalSpanNoRequestID})
}
