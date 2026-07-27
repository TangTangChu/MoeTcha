# 萌茶

MoeTcha，可以叫做萌茶或者萌查，一个基于个人兴趣爱好而捣鼓的简单的验证码服务

API文档是[这个](API.md)

普通验证码接口的图片编码仍遵循当前构建配置；`POST /grid/generate` 使用纯 Go WebP 编码路径，Windows 下也会返回真正的 WebP 图片。

不过也可以使用Docker compose
