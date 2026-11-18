package session

import (
	"github.com/alexedwards/scs/mysqlstore"
	"github.com/alexedwards/scs/postgresstore"
	"github.com/alexedwards/scs/sqlite3store"
	"github.com/alexedwards/scs/v2"
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/db"

	"time"
)

var Instance *scs.SessionManager

func Init() {
	Instance = scs.New()
	Instance.Lifetime = 7 * 24 * time.Hour
	Instance.Cookie.Name = "imail_session"
	Instance.Cookie.HttpOnly = true
	// Do not send the session cookie over plain HTTP when HTTPS is enabled.
	Instance.Cookie.Secure = config.Instance.HttpsEnabled != 2
	// Sessions are stored in the database; to keep the architecture simple
	// redis is not used for now. If performance becomes a bottleneck, sessions
	// can be moved to redis later.

	switch config.Instance.DbType {
	case config.DBTypeMySQL:
		Instance.Store = mysqlstore.New(db.Instance.DB().DB)
	case config.DBTypeSQLite:
		Instance.Store = sqlite3store.New(db.Instance.DB().DB)
	case config.DBTypePostgres:
		Instance.Store = postgresstore.New(db.Instance.DB().DB)
	default:
		panic("Unsupported database type: " + config.Instance.DbType)
	}

}
