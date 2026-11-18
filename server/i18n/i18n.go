package i18n

import "strings"

var (
	cn = map[string]string{
		"all_email":             "全部邮件数据",
		"inbox":                 "收件箱",
		"outbox":                "发件箱",
		"sketch":                "草稿箱",
		"aperror":               "用户名或密码错误",
		"login_requires_email":  "请以 @ 符号指定具体域名",
		"unknowError":           "未知错误",
		"succ":                  "成功",
		"send_fail":             "发送失败",
		"att_err":               "附件解码错误",
		"login_exp":             "登录已失效",
		"ip_taps":               "这是你服务器IP，确保这个IP正确",
		"invalid_email_address": "无效的邮箱地址！",
		"deleted":               "已删除",
		"junk":                  "广告箱",
	}
	tw = map[string]string{
		"all_email":             "全部郵件資料",
		"inbox":                 "收件匣",
		"outbox":                "寄件備份",
		"sketch":                "草稿",
		"aperror":               "使用者名稱或密碼錯誤",
		"login_requires_email":  "請以 @ 符號指定具體網域",
		"unknowError":           "未知錯誤",
		"succ":                  "成功",
		"send_fail":             "傳送失敗",
		"att_err":               "附件解碼錯誤",
		"login_exp":             "登入已失效",
		"ip_taps":               "這是你的伺服器 IP，請確認是否正確",
		"invalid_email_address": "無效的電子郵件地址",
		"deleted":               "已刪除",
		"junk":                  "垃圾郵件",
	}
	en = map[string]string{
		"all_email":             "All Mail",
		"inbox":                 "Inbox",
		"outbox":                "Sent",
		"sketch":                "Drafts",
		"aperror":               "Incorrect username or password",
		"login_requires_email":  "Specify the domain with @",
		"unknowError":           "Unknown error",
		"succ":                  "Success",
		"send_fail":             "Failed to send",
		"att_err":               "Attachment decoding error",
		"login_exp":             "Session expired",
		"ip_taps":               "This is your server's IP address. Make sure it is correct.",
		"invalid_email_address": "Invalid email address",
		"deleted":               "Deleted",
		"junk":                  "Junk",
	}
	fr = map[string]string{
		"all_email": "Tous les messages", "inbox": "Boîte de réception", "outbox": "Envoyés",
		"sketch": "Brouillons", "aperror": "Nom d'utilisateur ou mot de passe incorrect",
		"login_requires_email": "Indiquez le domaine avec @",
		"unknowError":          "Erreur inconnue", "succ": "Succès", "send_fail": "Échec de l'envoi",
		"att_err": "Erreur de décodage de la pièce jointe", "login_exp": "Session expirée",
		"ip_taps":               "Ceci est l'adresse IP de votre serveur. Vérifiez qu'elle est correcte.",
		"invalid_email_address": "Adresse e-mail invalide", "deleted": "Supprimé", "junk": "Indésirables",
	}
	es = map[string]string{
		"all_email": "Todo el correo", "inbox": "Bandeja de entrada", "outbox": "Enviados",
		"sketch": "Borradores", "aperror": "Nombre de usuario o contraseña incorrectos",
		"login_requires_email": "Especifica el dominio con @",
		"unknowError":          "Error desconocido", "succ": "Correcto", "send_fail": "Error al enviar",
		"att_err": "Error al decodificar el adjunto", "login_exp": "Sesión caducada",
		"ip_taps":               "Esta es la IP de tu servidor. Comprueba que sea correcta.",
		"invalid_email_address": "Dirección de correo no válida", "deleted": "Eliminado", "junk": "Correo no deseado",
	}
	ru = map[string]string{
		"all_email": "Вся почта", "inbox": "Входящие", "outbox": "Отправленные",
		"sketch": "Черновики", "aperror": "Неверное имя пользователя или пароль",
		"login_requires_email": "Укажите домен через @",
		"unknowError":          "Неизвестная ошибка", "succ": "Успешно", "send_fail": "Не удалось отправить",
		"att_err": "Ошибка декодирования вложения", "login_exp": "Сеанс истёк",
		"ip_taps":               "Это IP-адрес вашего сервера. Убедитесь, что он указан верно.",
		"invalid_email_address": "Неверный адрес электронной почты", "deleted": "Удалено", "junk": "Спам",
	}
	ar = map[string]string{
		"all_email": "كل البريد", "inbox": "الوارد", "outbox": "المرسل",
		"sketch": "المسودات", "aperror": "اسم المستخدم أو كلمة المرور غير صحيحة",
		"login_requires_email": "حدد النطاق باستخدام @",
		"unknowError":          "خطأ غير معروف", "succ": "تم بنجاح", "send_fail": "فشل الإرسال",
		"att_err": "خطأ في فك ترميز المرفق", "login_exp": "انتهت الجلسة",
		"ip_taps":               "هذا عنوان IP للخادم. تأكد من صحته.",
		"invalid_email_address": "عنوان بريد إلكتروني غير صالح", "deleted": "محذوف", "junk": "بريد مزعج",
	}
)

func GetText(lang, key string) string {
	var dict map[string]string
	switch strings.ToLower(strings.ReplaceAll(lang, "-", "")) {
	case "zhcn", "zh":
		dict = cn
	case "zhtw", "zhhk", "zhmo":
		dict = tw
	case "fr":
		dict = fr
	case "es":
		dict = es
	case "ru":
		dict = ru
	case "ar":
		dict = ar
	default:
		dict = en
	}
	if text, ok := dict[key]; ok {
		return text
	}
	return en[key]
}
