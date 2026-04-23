package auth

// this file is a demo for user_token_getter_url

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkauthen "github.com/larksuite/oapi-sdk-go/v3/service/authen/v1"
)

const (
	appID     = ""
	appSecret = ""
	redirectURI
	authHost = "open.feishu.cn/open.larksuite.com"
)

var (
	larkClient = lark.NewClient(appID, appSecret)
)

// UserAuth handles the initial step of the OAuth flow by redirecting the user to the Lark authorization page.
func UserAuth(ctx context.Context, c *app.RequestContext) {
	state := c.Query("state")
	if state == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "missing state"})
		return
	}

	scope := c.Query("scope")

	authURLObj, _ := url.Parse(fmt.Sprintf("https://%s/open-apis/authen/v1/index", authHost))
	query := authURLObj.Query()
	query.Set("redirect_uri", redirectURI)
	query.Set("app_id", appID)
	query.Set("state", state)
	if scope != "" {
		query.Set("scope", scope)
	}
	authURLObj.RawQuery = query.Encode()

	c.Redirect(http.StatusFound, []byte(authURLObj.String()))
}

// OAuthCallback processes the OAuth callback from Lark, fetches the user access token, and sends it back to the local server.
func OAuthCallback(ctx context.Context, c *app.RequestContext) {
	code := c.Query("code")
	state := c.Query("state")

	if code == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "missing code"})
		return
	}

	stateInt, err := strconv.Atoi(state)
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid state parameter, must be integer"})
		return
	}

	// get user_access_token
	req := larkauthen.NewCreateAccessTokenReqBuilder().
		Body(larkauthen.NewCreateAccessTokenReqBodyBuilder().
			GrantType("authorization_code").
			Code(code).
			Build()).
		Build()

	resp, err := larkClient.Authen.AccessToken.Create(ctx, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "failed to get access token", "detail": err.Error()})
		return
	}

	if !resp.Success() {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "feishu API returned error", "code": resp.Code, "msg": resp.Msg})
		return
	}

	data := resp.Data
	if data == nil || data.AccessToken == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "empty data in response"})
		return
	}

	// TODO check user permission or scope

	dataBytes, err := json.Marshal(data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "failed to marshal data"})
		return
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Lark Token</title>
</head>
<body>
    <p>Sending token...</p>
    <script>
        window.onload = function() {
            var url = 'http://127.0.0.1:%d/user_access_token';
            var data = %s;
            fetch(url, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(data)
            }).then(function(response) {
                document.body.innerHTML += '<p>Token sended，close in 3 seconds...</p>';
                setTimeout(function() {
                    window.close();
                }, 3000);
            }).catch(function(err) {
                document.body.innerHTML += '<p>Token send fail: ' + err + '</p>';
            });
        };
    </script>
</body>
</html>`, stateInt, string(dataBytes))

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}
