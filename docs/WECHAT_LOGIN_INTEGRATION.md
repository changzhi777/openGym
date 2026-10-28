# 微信扫码登录 · 小程序端对接文档（沐禾健身）

> 后端已实现（backend-go，2026-10-29）。本文档给 **qingmu2026 小程序项目**的开发者：
> 加一个「确认登录」页面即可打通 PC 网页的微信扫码登录。

## 流程总览

```
PC 网页 (app.qingmulife.com/login)
  │ ① GET /api/auth/wechat/qrcode
  │    ← { ticket, image }  image = 小程序码 PNG(base64)，scene 携带 t=<ticket>
  ▼
用户微信扫码 → 进入沐禾健身小程序确认页(scene 里的 ticket 带入)
  │ ② 小程序 wx.login() 拿 code
  │ ③ POST /api/auth/wechat/confirm { ticket, wxCode, invite? }
  │    后端 code2session 换 openid → 找/建账号 → 票据翻 confirmed
  ▼
PC 轮询 GET /api/auth/wechat/poll?ticket=...
  │  status: pending → confirmed(响应含 user 且已 Set-Cookie)
  ▼
PC 登录完成
```

## 后端端点（已实现，直接调）

| 端点 | 说明 |
|------|------|
| `POST /api/auth/wechat/confirm` | **小程序确认页调用**。body：`{ "ticket": "<scene 里的 t>", "wxCode": "<wx.login 的 code>", "invite": "<可选，邀请制实例建号用>" }`。成功 `{ ok: true }`；票据无效 404；`WX_MINI_*` 未配 503。 |
| `GET /api/auth/wechat/qrcode` | PC 侧出码（小程序端一般不用）。 |
| `GET /api/auth/wechat/poll?ticket=` | PC 轮询（小程序端不用）。 |

微信身份首扫自动建号（昵称 `沐禾用户<openid 尾4>`）；邀请制实例（invite_only）建号需带 `invite`。

## 小程序端确认页（可直接粘贴）

**pages/login-confirm/login-confirm.js**

```js
const BASE = 'https://app.qingmulife.com'   // 生产 API 同源

Page({
  data: { ticket: '', status: 'ready', name: '' },
  onLoad(q) {
    // 小程序码 scene 形如 "t=<ticket>"
    const ticket = (q.scene || '').split('&').map(s => s.split('='))
      .find(kv => kv[0] === 't')
    if (ticket) this.setData({ ticket: decodeURIComponent(ticket[1]) })
  },
  async confirm() {
    const { ticket } = this.data
    if (!ticket) return
    const { code } = await new Promise((ok, no) =>
      wx.login({ success: ok, fail: no }))
    wx.request({
      url: `${BASE}/api/auth/wechat/confirm`,
      method: 'POST',
      data: { ticket, wxCode: code },
      success: r => {
        if (r.statusCode === 200) {
          this.setData({ status: 'done' })
          setTimeout(() => wx.navigateBack(), 800)
        } else {
          this.setData({ status: 'fail', name: r.data.error || 'confirm failed' })
        }
      },
      fail: () => this.setData({ status: 'fail', name: 'network error' }),
    })
  },
})
```

**pages/login-confirm/login-confirm.wxml**

```xml
<view class="wrap">
  <view class="title">沐禾健身 · 扫码登录</view>
  <view class="tip">确认后，你正在使用的网页将自动登录。</view>
  <button class="btn" bindtap="confirm" disabled="{{ status === 'done' }}">
    {{ status === 'done' ? '已确认 ✓' : '确认登录' }}
  </button>
  <view class="err" wx-if="{{ status === 'fail' }}">{{ name }}</view>
</view>
```

**样式**（login-confirm.wxss）按小程序项目规范自定；确认按钮主色 `#0c0e12` 底/白字与品牌一致。

## 配置与联调

1. **后端 env**（118 已预留，凭据从小程序控制台取）：
   - `WX_MINI_APPID` = 小程序 AppID（wx 开头）
   - `WX_MINI_SECRET` = 小程序 AppSecret
   - 未配置时 confirm/qrcode 答 503（`wechat-off`）
2. **小程序码 page 路径**：后端 getUnlimitedQRCode 的 `page` 固定为 `pages/login-confirm/login-confirm`
   —— **小程序端必须创建该路径页面**，否则出码接口报 41030（page 不存在）。
3. **scene 长度**：ticket 约 30 字符，`t=<ticket>` 在 getUnlimitedQRCode 的 32 字符 scene 限制内。
4. **联调顺序**：后端配 env 重启 → 小程序开发者工具编译到确认页（手动传 scene）→ 确认 → PC /login 出码端到端。
5. **安全说明**：confirm 以「谁扫码谁确认」为信任源（openid 绑定），ticket 一次性、5 分钟过期；
   邀请制实例建号需邀请码。

## 常见问题

| 现象 | 原因 |
|------|------|
| confirm 503 wechat-off | 后端 WX_MINI_APPID/SECRET 未配 |
| confirm 401 wechat: 40029 | wxCode 无效/已用（wx.login 的 code 一次性，确认页每次重新 login） |
| qrcode 502 | getUnlimitedQRCode 失败——常见 41030 page 路径不存在 |
| PC 一直 pending | confirm 未成功（看小程序端请求与后端日志） |
