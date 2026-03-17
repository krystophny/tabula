package ews

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientUpdateInboxRulesBuildsOperations(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/" xmlns:m="http://schemas.microsoft.com/exchange/services/2006/messages">
  <soap:Body>
    <m:UpdateInboxRulesResponse>
      <m:ResponseCode>NoError</m:ResponseCode>
    </m:UpdateInboxRulesResponse>
  </soap:Body>
</soap:Envelope>`)
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Endpoint: server.URL,
		Username: "ert",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	defer client.Close()

	err = client.UpdateInboxRules(t.Context(), []RuleOperation{
		{
			Kind: RuleOperationCreate,
			Rule: Rule{
				Name:     "Move project mail",
				Priority: 1,
				Enabled:  true,
				Conditions: RuleConditions{
					ContainsSubjectStrings: []string{"project"},
				},
				Actions: RuleActions{
					MoveToFolderID:      "inbox",
					StopProcessingRules: true,
				},
			},
		},
		{
			Kind: RuleOperationDelete,
			Rule: Rule{ID: "rule-1"},
		},
	})
	if err != nil {
		t.Fatalf("UpdateInboxRules() error: %v", err)
	}

	for _, snippet := range []string{
		"<m:UpdateInboxRules>",
		"<t:CreateRuleOperation>",
		"<t:ContainsSubjectStrings><t:String>project</t:String></t:ContainsSubjectStrings>",
		"<t:MoveToFolder><t:DistinguishedFolderId Id=\"inbox\" /></t:MoveToFolder>",
		"<t:DeleteRuleOperation><t:RuleId>rule-1</t:RuleId></t:DeleteRuleOperation>",
	} {
		if !strings.Contains(body, snippet) {
			t.Fatalf("request body missing %q:\n%s", snippet, body)
		}
	}
}

func TestClientGetMessagesSanitizesIllegalXMLCharacters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = io.WriteString(w, "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n"+
			"<soap:Envelope xmlns:soap=\"http://schemas.xmlsoap.org/soap/envelope/\" xmlns:m=\"http://schemas.microsoft.com/exchange/services/2006/messages\" xmlns:t=\"http://schemas.microsoft.com/exchange/services/2006/types\">\n"+
			"  <soap:Body>\n"+
			"    <m:GetItemResponse>\n"+
			"      <m:ResponseMessages>\n"+
			"        <m:GetItemResponseMessage>\n"+
			"          <m:ResponseCode>NoError</m:ResponseCode>\n"+
			"          <m:Items>\n"+
			"            <t:Message>\n"+
			"              <t:ItemId Id=\"msg-1\" ChangeKey=\"ck-1\" />\n"+
			"              <t:ParentFolderId Id=\"inbox\" ChangeKey=\"fold-1\" />\n"+
			"              <t:ConversationId Id=\"thread-1\" ChangeKey=\"conv-1\" />\n"+
			"              <t:Subject>Hello\x1b World</t:Subject>\n"+
			"              <t:Body BodyType=\"Text\">Body\x1b text</t:Body>\n"+
			"              <t:DateTimeReceived>2026-03-16T14:00:00Z</t:DateTimeReceived>\n"+
			"              <t:IsRead>false</t:IsRead>\n"+
			"            </t:Message>\n"+
			"          </m:Items>\n"+
			"        </m:GetItemResponseMessage>\n"+
			"      </m:ResponseMessages>\n"+
			"    </m:GetItemResponse>\n"+
			"  </soap:Body>\n"+
			"</soap:Envelope>")
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Endpoint: server.URL,
		Username: "ert",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	defer client.Close()

	messages, err := client.GetMessages(t.Context(), []string{"msg-1"})
	if err != nil {
		t.Fatalf("GetMessages() error: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	if messages[0].Subject != "Hello World" {
		t.Fatalf("Subject = %q, want %q", messages[0].Subject, "Hello World")
	}
	if messages[0].Body != "Body text" {
		t.Fatalf("Body = %q, want %q", messages[0].Body, "Body text")
	}
}

func TestClientGetMessagesSanitizesIllegalXMLCharacterReferences(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = io.WriteString(w, "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n"+
			"<soap:Envelope xmlns:soap=\"http://schemas.xmlsoap.org/soap/envelope/\" xmlns:m=\"http://schemas.microsoft.com/exchange/services/2006/messages\" xmlns:t=\"http://schemas.microsoft.com/exchange/services/2006/types\">\n"+
			"  <soap:Body>\n"+
			"    <m:GetItemResponse>\n"+
			"      <m:ResponseMessages>\n"+
			"        <m:GetItemResponseMessage>\n"+
			"          <m:ResponseCode>NoError</m:ResponseCode>\n"+
			"          <m:Items>\n"+
			"            <t:Message>\n"+
			"              <t:ItemId Id=\"msg-1\" ChangeKey=\"ck-1\" />\n"+
			"              <t:ParentFolderId Id=\"inbox\" ChangeKey=\"fold-1\" />\n"+
			"              <t:ConversationId Id=\"thread-1\" ChangeKey=\"conv-1\" />\n"+
			"              <t:Subject>Hello&#x1B; World</t:Subject>\n"+
			"              <t:Body BodyType=\"Text\">Body&#27; text</t:Body>\n"+
			"              <t:DateTimeReceived>2026-03-16T14:00:00Z</t:DateTimeReceived>\n"+
			"              <t:IsRead>false</t:IsRead>\n"+
			"            </t:Message>\n"+
			"          </m:Items>\n"+
			"        </m:GetItemResponseMessage>\n"+
			"      </m:ResponseMessages>\n"+
			"    </m:GetItemResponse>\n"+
			"  </soap:Body>\n"+
			"</soap:Envelope>")
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Endpoint: server.URL,
		Username: "ert",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	defer client.Close()

	messages, err := client.GetMessages(t.Context(), []string{"msg-1"})
	if err != nil {
		t.Fatalf("GetMessages() error: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	if messages[0].Subject != "Hello World" {
		t.Fatalf("Subject = %q, want %q", messages[0].Subject, "Hello World")
	}
	if messages[0].Body != "Body text" {
		t.Fatalf("Body = %q, want %q", messages[0].Body, "Body text")
	}
}

func TestClientGetMessageSummariesRequestsMetadataShape(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/" xmlns:m="http://schemas.microsoft.com/exchange/services/2006/messages" xmlns:t="http://schemas.microsoft.com/exchange/services/2006/types">
  <soap:Body>
    <m:GetItemResponse>
      <m:ResponseMessages>
        <m:GetItemResponseMessage ResponseClass="Success">
          <m:ResponseCode>NoError</m:ResponseCode>
          <m:Items>
            <t:Message>
              <t:ItemId Id="msg-1" ChangeKey="ck-1" />
              <t:ParentFolderId Id="inbox" ChangeKey="fold-1" />
              <t:ConversationId Id="thread-1" ChangeKey="conv-1" />
              <t:Subject>Hello World</t:Subject>
              <t:DateTimeReceived>2026-03-16T14:00:00Z</t:DateTimeReceived>
              <t:IsRead>false</t:IsRead>
            </t:Message>
          </m:Items>
        </m:GetItemResponseMessage>
      </m:ResponseMessages>
    </m:GetItemResponse>
  </soap:Body>
</soap:Envelope>`)
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Endpoint: server.URL,
		Username: "ert",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	defer client.Close()

	messages, err := client.GetMessageSummaries(t.Context(), []string{"msg-1"})
	if err != nil {
		t.Fatalf("GetMessageSummaries() error: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	if messages[0].Body != "" {
		t.Fatalf("Body = %q, want empty summary body", messages[0].Body)
	}
	if !strings.Contains(body, `<t:BaseShape>IdOnly</t:BaseShape>`) {
		t.Fatalf("request body missing IdOnly base shape: %s", body)
	}
	if strings.Contains(body, `<t:BodyType>`) {
		t.Fatalf("request body unexpectedly requested body content: %s", body)
	}
	if !strings.Contains(body, `FieldURI="item:Subject"`) {
		t.Fatalf("request body missing subject metadata field: %s", body)
	}
}

func TestClientMoveItemsReturnsResolvedIDs(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/" xmlns:m="http://schemas.microsoft.com/exchange/services/2006/messages" xmlns:t="http://schemas.microsoft.com/exchange/services/2006/types">
  <soap:Body>
    <m:MoveItemResponse>
      <m:ResponseMessages>
        <m:MoveItemResponseMessage ResponseClass="Success">
          <m:ResponseCode>NoError</m:ResponseCode>
          <m:Items>
            <t:Message><t:ItemId Id="m1-new" ChangeKey="ck1" /></t:Message>
          </m:Items>
        </m:MoveItemResponseMessage>
        <m:MoveItemResponseMessage ResponseClass="Success">
          <m:ResponseCode>NoError</m:ResponseCode>
          <m:Items>
            <t:Message><t:ItemId Id="m2-new" ChangeKey="ck2" /></t:Message>
          </m:Items>
        </m:MoveItemResponseMessage>
      </m:ResponseMessages>
    </m:MoveItemResponse>
  </soap:Body>
</soap:Envelope>`)
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Endpoint: server.URL,
		Username: "ert",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	defer client.Close()

	ids, err := client.MoveItems(t.Context(), []string{"m1", "m2"}, "deleteditems")
	if err != nil {
		t.Fatalf("MoveItems() error: %v", err)
	}
	if strings.Join(ids, ",") != "m1-new,m2-new" {
		t.Fatalf("resolved ids = %v, want [m1-new m2-new]", ids)
	}
	if !strings.Contains(body, `<t:DistinguishedFolderId Id="deleteditems" />`) {
		t.Fatalf("MoveItem body missing deleteditems folder: %s", body)
	}
}

func TestClientCallParsesServerBusyBackoffFault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Body>
    <s:Fault>
      <faultcode xmlns:a="http://schemas.microsoft.com/exchange/services/2006/types">a:ErrorServerBusy</faultcode>
      <faultstring xml:lang="de-AT">The server cannot service this request right now. Try again later.</faultstring>
      <detail>
        <e:ResponseCode xmlns:e="http://schemas.microsoft.com/exchange/services/2006/errors">ErrorServerBusy</e:ResponseCode>
        <e:Message xmlns:e="http://schemas.microsoft.com/exchange/services/2006/errors">The server cannot service this request right now. Try again later.</e:Message>
        <t:MessageXml xmlns:t="http://schemas.microsoft.com/exchange/services/2006/types">
          <t:Value Name="BackOffMilliseconds">12345</t:Value>
        </t:MessageXml>
      </detail>
    </s:Fault>
  </s:Body>
</s:Envelope>`)
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Endpoint: server.URL,
		Username: "ert",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	defer client.Close()

	err = client.UpdateInboxRules(t.Context(), nil)
	if err == nil {
		t.Fatalf("UpdateInboxRules() error = nil, want server busy error")
	}
	var backoffErr *BackoffError
	if !strings.Contains(err.Error(), "retry after") {
		t.Fatalf("error = %v, want retry-after message", err)
	}
	if got := err; got != nil {
		var ok bool
		backoffErr, ok = got.(*BackoffError)
		if !ok {
			t.Fatalf("error type = %T, want *BackoffError", err)
		}
	}
	if backoffErr.Backoff != 12345*time.Millisecond {
		t.Fatalf("Backoff = %v, want %v", backoffErr.Backoff, 12345*time.Millisecond)
	}
}
