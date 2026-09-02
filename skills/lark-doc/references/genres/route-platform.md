# Genre Router: Platform / 平台发布稿 (`route_platform`)

仅当最终交付物是小红书笔记、微信公众号文章或邮件成稿时进入本 router；按目标平台选择且只读取一个 leaf。仅把平台作为研究对象、信息来源或业务渠道时不触发；多平台成稿分别路由和生成。

| 关键词             | Leaf                               |
|-----------------|------------------------------------|
| XHS、小红书         | [`xiaohongshu.md`](xiaohongshu.md) |
| 微信、wechat       | [`wechat.md`](wechat.md)           |
| 邮件、email、e-mail | [`email.md`](email.md)             |
