// Package hardcover wraps the Hardcover.app GraphQL API for pushing
// reading progress on behalf of the user. The endpoint is
// https://api.hardcover.app/v1/graphql and authenticates with a
// Bearer token the user obtains from their Hardcover profile.
//
// The package is intentionally narrow — one function per use case
// (PushStatus, PushProgress) rather than a generic GraphQL client.
// Mutations are documented in code with the exact GraphQL operation
// the user can copy into Hardcover's Explorer to verify behaviour.
package hardcover

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const endpoint = "https://api.hardcover.app/v1/graphql"

type Client struct {
	hc    *http.Client
	token string
}

func New(token string) *Client {
	return &Client{
		hc:    &http.Client{Timeout: 30 * time.Second},
		token: strings.TrimSpace(token),
	}
}

// graphqlRequest is the standard {query, variables} envelope. Used
// for every Hardcover call.
type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type graphqlResponse struct {
	Data   json.RawMessage   `json:"data"`
	Errors []graphqlErrorMsg `json:"errors,omitempty"`
}

type graphqlErrorMsg struct {
	Message string `json:"message"`
}

func (c *Client) do(ctx context.Context, query string, vars map[string]any, out any) error {
	if c.token == "" {
		return errors.New("hardcover token not configured")
	}
	body, err := json.Marshal(graphqlRequest{Query: query, Variables: vars})
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("hardcover %d: %s", resp.StatusCode,
			strings.TrimSpace(string(snippet)))
	}
	var parsed graphqlResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 5<<20)).Decode(&parsed); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	if len(parsed.Errors) > 0 {
		msgs := make([]string, 0, len(parsed.Errors))
		for _, e := range parsed.Errors {
			msgs = append(msgs, e.Message)
		}
		return fmt.Errorf("hardcover graphql: %s", strings.Join(msgs, "; "))
	}
	if out != nil && len(parsed.Data) > 0 {
		if err := json.Unmarshal(parsed.Data, out); err != nil {
			return fmt.Errorf("decode data: %w", err)
		}
	}
	return nil
}

// AuthCheck pings the `me` query to verify the token works.
// Returns nil + the username on success.
func (c *Client) AuthCheck(ctx context.Context) (string, error) {
	const query = `query { me { username } }`
	var out struct {
		Me []struct {
			Username string `json:"username"`
		} `json:"me"`
	}
	if err := c.do(ctx, query, nil, &out); err != nil {
		return "", err
	}
	if len(out.Me) == 0 {
		return "", errors.New("hardcover me query returned no user")
	}
	return out.Me[0].Username, nil
}

// PushBookStatus sets the user's status on one book (identified by
// Hardcover's edition id). status is one of "want_to_read",
// "currently_reading", "read". Returns the user_book id on success.
//
// GraphQL:
//   mutation ($editionId: Int!, $status: Int!) {
//     insert_user_book(object: {edition_id: $editionId, status_id: $status}) { id }
//   }
//
// Hardcover encodes the status as an int (1=want, 2=current, 3=read).
func (c *Client) PushBookStatus(ctx context.Context, editionID int, status string) (int, error) {
	statusInt := 2
	switch status {
	case "want_to_read", "want", "wishlist":
		statusInt = 1
	case "read", "finished", "complete":
		statusInt = 3
	}
	const query = `
		mutation ($editionId: Int!, $status: Int!) {
		  insert_user_book(object: {edition_id: $editionId, status_id: $status}) {
		    id
		  }
		}`
	var out struct {
		InsertUserBook struct {
			ID int `json:"id"`
		} `json:"insert_user_book"`
	}
	if err := c.do(ctx, query, map[string]any{
		"editionId": editionID,
		"status":    statusInt,
	}, &out); err != nil {
		return 0, err
	}
	return out.InsertUserBook.ID, nil
}

// LookupByISBN searches Hardcover for an edition matching the given
// ISBN-13. Returns the edition id (0 + nil on no match) so callers
// can pass it to PushBookStatus without a separate lookup hop.
//
// GraphQL:
//   query ($isbn: String!) {
//     editions(where: {isbn_13: {_eq: $isbn}}, limit: 1) { id }
//   }
func (c *Client) LookupByISBN(ctx context.Context, isbn string) (int, error) {
	isbn = strings.ReplaceAll(strings.TrimSpace(isbn), "-", "")
	if isbn == "" {
		return 0, errors.New("isbn required")
	}
	const query = `
		query ($isbn: String!) {
		  editions(where: {isbn_13: {_eq: $isbn}}, limit: 1) { id }
		}`
	var out struct {
		Editions []struct {
			ID int `json:"id"`
		} `json:"editions"`
	}
	if err := c.do(ctx, query, map[string]any{"isbn": isbn}, &out); err != nil {
		return 0, err
	}
	if len(out.Editions) == 0 {
		return 0, nil
	}
	return out.Editions[0].ID, nil
}
