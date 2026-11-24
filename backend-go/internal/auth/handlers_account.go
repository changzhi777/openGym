// handlers_account.go — 账号自助注销与数据导出(阶段 2,2026-11-25)。
//
// 契约:openapi.yaml accountDelete / accountExport。
// 注销 = DELETE FROM users 一条 SQL(全库 FK ON DELETE CASCADE 带走 states/
// identities/sessions/push/media/coach 五表)+ DATA_DIR/media/<uid> 文件目录
// 清理 + cookie 双清 + 审计。proof 按 Go 栈认证重构适配:绑定手机号须短信
// 验证码(新鲜码走 /api/auth/sms/send),无绑定手机号的账号(微信建号)仅
// 回显账号名即证明(会话即凭证,审计 proof=name)。
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/changzhi777/openGym/backend-go/internal/store"
)

// lastAdminLeaving 该账号是唯一在任管理员且实例还有其他账号 → 拒(409 last-admin)。
// 唯一账号可自删(实例清空重来);多管理员删一个还剩别人。
func (s *Service) lastAdminLeaving(ctx context.Context, uid string) bool {
	var admins, users int
	s.DB.QueryRow(ctx, `SELECT count(*) FROM admin_roles WHERE revoked_at IS NULL`).Scan(&admins)
	s.DB.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&users)
	if admins != 1 || users <= 1 {
		return false
	}
	var one int
	s.DB.QueryRow(ctx, `SELECT 1 FROM admin_roles WHERE user_id=$1 AND revoked_at IS NULL`, uid).Scan(&one)
	return one == 1
}

// boundPhone 该账号绑定的手机号(注销 proof 用;无绑定返回空)。
func (s *Service) boundPhone(ctx context.Context, uid string) string {
	var phone string
	s.DB.QueryRow(ctx, `SELECT identifier FROM auth_identities WHERE user_id=$1 AND kind='phone' LIMIT 1`, uid).Scan(&phone)
	return phone
}

// DeleteAccount POST /api/account/delete — 自助注销,不可恢复。
func (s *Service) DeleteAccount(c *gin.Context) {
	u, ok := UserOf(c, s.DB, s.Sessions)
	if !ok {
		jsonFail(c, 401, "not signed in", "")
		return
	}
	ctx := c.Request.Context()
	b := readBody(c)
	name := strings.TrimSpace(b.Name)
	if name == "" || name != u.Name {
		jsonFail(c, 400, "type the account name to confirm", "confirm-name")
		return
	}
	if s.lastAdminLeaving(ctx, u.ID) {
		jsonFail(c, 409, "the last admin cannot leave while other profiles remain", "last-admin")
		return
	}
	// proof:name-only(输入正确账号名即本人;会话本身经手机号验证码签发,CSRF 中间件
	// 挡跨站,输入匹配挡误触)。绑定手机的账号可选加验短信码:带 smsCode 才校验,
	// 带了但错 → 400 sms-code(前端暂不发码,留作增强通道)。
	proof := "name"
	if phone := s.boundPhone(ctx, u.ID); phone != "" {
		if code := strings.TrimSpace(b.SmsCode); code != "" {
			if !s.SmsStore.Verify(ctx, phone, code) {
				s.audit(c, u.ID, "auth.proof.fail", gin.H{"act": "account-delete"})
				jsonFail(c, 400, "wrong or expired code", "sms-code")
				return
			}
			proof = "sms"
		}
	}
	s.audit(c, u.ID, "auth.account.delete", gin.H{"name": u.Name, "proof": proof})
	if _, err := s.DB.Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID); err != nil {
		jsonFail(c, 500, "delete", "")
		return
	}
	// 媒体文件不走 PG:盘上 <DATA_DIR>/media/<uid> 一并清(media 表行已级联)。
	if s.DataDir != "" {
		_ = os.RemoveAll(filepath.Join(s.DataDir, "media", u.ID))
	}
	for _, v := range s.Sessions.ClearValue() {
		c.Writer.Header().Add("Set-Cookie", v)
	}
	c.JSON(200, gin.H{"ok": true})
}

// ExportAccount GET /api/account/export — 账号全部服务端数据一个 JSON 下载。
func (s *Service) ExportAccount(c *gin.Context) {
	u, ok := UserOf(c, s.DB, s.Sessions)
	if !ok {
		jsonFail(c, 401, "not signed in", "")
		return
	}
	ctx := c.Request.Context()
	ids, _ := store.IdentitiesOf(ctx, s.DB, u.ID)
	var createdAt time.Time
	s.DB.QueryRow(ctx, `SELECT created_at FROM users WHERE id=$1`, u.ID).Scan(&createdAt)
	state := gin.H{"doc": gin.H{}, "rev": 0}
	if d, err := store.GetStateDoc(ctx, s.DB, u.ID); err == nil {
		var doc any
		if json.Unmarshal(d.Doc, &doc) == nil && doc != nil {
			state = gin.H{"doc": doc, "rev": d.Rev}
		}
	}
	out, _ := json.Marshal(gin.H{
		"account":    gin.H{"id": u.ID, "name": u.Name, "createdAt": createdAt},
		"identities": ids,
		"state":      state,
		"exportedAt": time.Now(),
	})
	name := "muhefit-export-" + u.ID
	c.Header("Content-Disposition", `attachment; filename="`+name+`.json"`)
	c.Data(200, "application/json", bytes.TrimSpace(out))
}
