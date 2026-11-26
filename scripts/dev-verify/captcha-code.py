import sys, json, re

# captcha-code.py — stdin 吃 /api/auth/captcha 的 JSON,stdout 出 4 位验证码。
# SVG 里数字是 <text x="..">N</text> 明文,按 x 升序拼接即答案(服务端生成序=位置序)。
d = json.load(sys.stdin)
svg = d["svg"]
xs = re.findall(r'<text x="(\d+)"[^>]*?>(\d)</text>', svg)
print("".join(dg for _, dg in sorted(xs, key=lambda t: int(t[0]))))
