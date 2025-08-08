package pgconn

import (
	"bytes"
	"fmt"

	"github.com/jackc/pgx/v5/pgproto3"
)

func (c *PgConn) rxOAuthSASLOk() (*pgproto3.AuthenticationOk, error) {
	msg, err := c.receiveMessage()
	if err != nil {
		return nil, err
	}
	switch m := msg.(type) {
	case *pgproto3.AuthenticationOk:
		return m, nil
	// AuthenticationSASLContinue is received in when the token is empty or invalid in SASLInitialResponse phase
	//case *pgproto3.AuthenticationSASLContinue:
	//		return nil, fmt.Errorf(": %s", string(m.Data))
	case *pgproto3.ErrorResponse:
		return nil, ErrorResponseToPgError(m)
	}

	return nil, fmt.Errorf("expected AuthenticationOk message but received unexpected %T", msg)
}

// simply send SASLInitialClientResponse directly with oauth token
func (c *PgConn) oauthAuth() error {
	// Check if we have a pre-configured bearer token
	if c.config == nil || c.config.OAuthBearerToken == "" {
		return fmt.Errorf("server requires OAuthBearerToken for this connection")
	}

	// Construct a SASLInitialResponse message
	reply := &pgproto3.SASLInitialResponse{
		AuthMechanism: "OAUTHBEARER",
		Data:          buildOAuthInitialResponse(c.config.OAuthBearerToken),
	}

	// Use frontend.Send() to populate the send buffer and flush it to the wire
	c.frontend.Send(reply)
	if err := c.flushWithPotentialWriteReadDeadlock(); err != nil {
		return err
	}

	// Wait for AuthenticationOk message
	_, err := c.rxOAuthSASLOk()
	if err != nil {
		return err
	}

	return nil
}

// buildOAuthInitialResponse creates RFC 7628 OAUTHBEARER initial client response：
//
// from  4.1.  Successful Bearer Token Exchange
//
//	[Initial connection and TLS establishment...]
//	S: * OK IMAP4rev1 Server Ready
//	C: t0 CAPABILITY
//	S: * CAPABILITY IMAP4rev1 AUTH=OAUTHBEARER SASL-IR
//	S: t0 OK Completed
//	C: t1 AUTHENTICATE OAUTHBEARER bixhPXVzZXJAZXhhbXBsZS5jb20sAWhv
//	      c3Q9c2VydmVyLmV4YW1wbGUuY29tAXBvcnQ9MTQzAWF1dGg9QmVhcmVyI
//	      HZGOWRmdDRxbVRjMk52YjNSbGNrQmhiSFJoZG1semRHRXVZMjl0Q2c9PQ
//	      EB
//	S: t1 OK SASL authentication succeeded
//
//	As required by IMAP [RFC3501], the payloads are base64 encoded.  The
//	decoded initial client response (with %x01 represented as ^A and long
//	lines wrapped for readability) is:
//
//	n,a=user@example.com,^Ahost=server.example.com^Aport=143^A
//	auth=Bearer vF9dft4qmTc2Nvb3RlckBhbHRhdmlzdGEuY29tCg==^A^A
//
// while the PG 18 beta2 code (libpq/auth-oauth.c)
//
// ```c
//
//	/* All remaining fields are separated by the RFC's kvsep (\x01). */
//	if (*p != KVSEP)
//		ereport(ERROR,
//				errcode(ERRCODE_PROTOCOL_VIOLATION),
//				errmsg("malformed OAUTHBEARER message"),
//				errdetail("Key-value separator expected, but found character \"%s\".",
//						  sanitize_char(*p)));
//	p++;
//
//	auth = parse_kvpairs_for_auth(&p);
//	if (!auth)
//		ereport(ERROR,
//				errcode(ERRCODE_PROTOCOL_VIOLATION),
//				errmsg("malformed OAUTHBEARER message"),
//				errdetail("Message does not contain an auth value."));
//
// ```
//
// The host and port are not requried according to the RFC, so
// we just send (note the <token> should be base64 encoded)
//
// ```
//
//	n,,^Aauth=Bearer <token>^A^A
//
// ```
// where ^A=0x01
func buildOAuthInitialResponse(token string) []byte {
	var b bytes.Buffer
	b.WriteString("n,,")
	b.WriteByte(0x01)
	b.WriteString("auth=Bearer ")
	b.WriteString(token)
	b.WriteByte(0x01)
	b.WriteByte(0x01)
	return b.Bytes()
}
