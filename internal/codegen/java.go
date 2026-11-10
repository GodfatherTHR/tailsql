// Package codegen renders ready-to-run Java connectivity artifacts for a
// TailSQL instance in either local or Tailscale mode.
package codegen

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// Mode selects the JDBC endpoint family.
type Mode string

const (
	ModeLocal     Mode = "local"
	ModeTailscale Mode = "tailscale"
)

// H2DriverVersion and HikariVersion are pinned generator dependencies.
const (
	H2DriverVersion   = "2.3.232"
	HikariVersion     = "6.2.1"
	SLF4JVersion      = "2.0.16"
	MaxPoolSize       = 5
	ConnectionTimeout = 10_000
)

var databaseNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)
var hostnamePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]{0,252})$`)

// Request carries the instance details selected by the IPC layer.
type Request struct {
	InstanceID string
	Mode       Mode
	Database   string
	Username   string
	Password   string
	Host       string
	Port       int
	MagicDNS   string
	Available  bool
	Reason     string
}

// Artifacts is the payload copied into the frontend code tabs.
type Artifacts struct {
	InstanceID       string `json:"instanceId"`
	Mode             string `json:"mode"`
	Available        bool   `json:"available"`
	Reason           string `json:"reason"`
	JDBCURL          string `json:"jdbcUrl"`
	Host             string `json:"host"`
	Port             int    `json:"port"`
	Database         string `json:"database"`
	Username         string `json:"username"`
	Password         string `json:"password"`
	MagicDNS         string `json:"magicDns,omitempty"`
	Maven            string `json:"maven"`
	Gradle           string `json:"gradle"`
	Hikari           string `json:"hikari"`
	SpringProperties string `json:"springProperties"`
	SpringYAML       string `json:"springYaml"`
	JDBCTest         string `json:"jdbcTest"`
}

// Generate renders all artifacts or returns an unavailable result with a
// human-readable reason.
func Generate(request Request) Artifacts {
	artifacts := Artifacts{
		InstanceID: request.InstanceID,
		Mode:       string(request.Mode),
		Database:   request.Database,
		Username:   request.Username,
		Password:   request.Password,
		Host:       request.Host,
		Port:       request.Port,
		MagicDNS:   sanitizeHostname(request.MagicDNS),
		Available:  request.Available,
		Reason:     request.Reason,
	}
	if !request.Available {
		if artifacts.Reason == "" {
			artifacts.Reason = "This connection mode is not available yet."
		}
		return artifacts
	}

	if err := validate(request); err != nil {
		artifacts.Available = false
		artifacts.Reason = err.Error()
		return artifacts
	}

	artifacts.JDBCURL = fmt.Sprintf("jdbc:h2:tcp://%s:%d/./%s", request.Host, request.Port, request.Database)
	artifacts.Maven = renderMaven(request)
	artifacts.Gradle = renderGradle(request)
	artifacts.Hikari = renderHikari(request, artifacts.JDBCURL, artifacts.MagicDNS)
	artifacts.SpringProperties = renderSpringProperties(request, artifacts.JDBCURL)
	artifacts.SpringYAML = renderSpringYAML(request, artifacts.JDBCURL, artifacts.MagicDNS)
	artifacts.JDBCTest = renderJDBCTest(request, artifacts.JDBCURL, artifacts.MagicDNS)
	return artifacts
}

func validate(request Request) error {
	if !databaseNamePattern.MatchString(request.Database) {
		return fmt.Errorf("database name %q is not a valid TailSQL slug", request.Database)
	}
	if strings.TrimSpace(request.Username) == "" {
		return fmt.Errorf("username is required")
	}
	if request.Port < 1 || request.Port > 65535 {
		return fmt.Errorf("port %d is invalid", request.Port)
	}
	ip := net.ParseIP(strings.TrimSpace(request.Host))
	if ip == nil || ip.To4() == nil {
		return fmt.Errorf("host %q is not an IPv4 address", request.Host)
	}
	if ip.IsUnspecified() {
		return fmt.Errorf("wildcard hosts are not permitted in generated URLs")
	}
	switch request.Mode {
	case ModeLocal:
		if !ip.IsLoopback() {
			return fmt.Errorf("local mode must use a loopback address")
		}
	case ModeTailscale:
		if !isTailscaleIPv4(ip) {
			return fmt.Errorf("Tailscale mode must use a 100.64.0.0/10 address")
		}
	default:
		return fmt.Errorf("unknown connection mode %q", request.Mode)
	}
	return nil
}

func isTailscaleIPv4(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	return v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127
}

func renderMaven(request Request) string {
	return fmt.Sprintf(`<!-- TailSQL — H2 + HikariCP for %s (%s mode) -->
<!-- Java 11+ required (Java 17/21 recommended). -->
<dependencies>
    <dependency>
        <groupId>com.h2database</groupId>
        <artifactId>h2</artifactId>
        <version>%s</version>
    </dependency>
    <dependency>
        <groupId>com.zaxxer</groupId>
        <artifactId>HikariCP</artifactId>
        <version>%s</version>
    </dependency>
    <dependency>
        <groupId>org.slf4j</groupId>
        <artifactId>slf4j-simple</artifactId>
        <version>%s</version>
        <scope>runtime</scope>
    </dependency>
</dependencies>
`, xmlEscape(request.Database), request.Mode, H2DriverVersion, HikariVersion, SLF4JVersion)
}

func renderGradle(request Request) string {
	return fmt.Sprintf(`// TailSQL — H2 + HikariCP for %s (%s mode)
// Java 11+ required (Java 17/21 recommended).
dependencies {
    implementation 'com.h2database:h2:%s'
    implementation 'com.zaxxer:HikariCP:%s'
    runtimeOnly 'org.slf4j:slf4j-simple:%s'
}
`, javaCommentSafe(request.Database), request.Mode, H2DriverVersion, HikariVersion, SLF4JVersion)
}

func renderHikari(request Request, jdbcURL, magicDNS string) string {
	magicDNSComment := ""
	if magicDNS != "" {
		magicDNSComment = fmt.Sprintf("\n * MagicDNS alternative: jdbc:h2:tcp://%s:%d/./%s", magicDNS, request.Port, request.Database)
	}
	return fmt.Sprintf(`import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;

import java.sql.Connection;
import java.sql.ResultSet;
import java.sql.Statement;

/**
 * TailSQL connection pool for "%s" (%s mode).
 * JDBC URL: %s%s
 *
 * Run main() to verify connectivity before using the pool in your service.
 */
public final class DatabaseConfig {

    private static final String JDBC_URL = %s;
    private static final String USER = %s;
    private static final String PASSWORD = %s;

    private DatabaseConfig() {
    }

    public static HikariDataSource createDataSource() {
        HikariConfig config = new HikariConfig();
        config.setJdbcUrl(JDBC_URL);
        config.setUsername(USER);
        config.setPassword(PASSWORD);
        config.setDriverClassName("org.h2.Driver");

        config.setMaximumPoolSize(%d);
        config.setMinimumIdle(1);
        config.setConnectionTimeout(%d);
        config.setValidationTimeout(5_000);
        config.setIdleTimeout(60_000);
        config.setMaxLifetime(300_000);
        config.setInitializationFailTimeout(5_000);
        config.setPoolName("tailsql-%s");

        return new HikariDataSource(config);
    }

    public static void main(String[] args) throws Exception {
        try (HikariDataSource dataSource = createDataSource();
             Connection connection = dataSource.getConnection();
             Statement statement = connection.createStatement();
             ResultSet rs = statement.executeQuery("SELECT 1")) {
            rs.next();
            System.out.println("Connected through TailSQL (HikariCP): SELECT 1 -> " + rs.getInt(1));
        }
    }
}
`, javaCommentSafe(request.Database), request.Mode, javaCommentSafe(jdbcURL), javaCommentSafe(magicDNSComment),
		javaQuote(jdbcURL), javaQuote(request.Username), javaQuote(request.Password),
		MaxPoolSize, ConnectionTimeout, poolName(request.Database))
}

func renderSpringProperties(request Request, jdbcURL string) string {
	return fmt.Sprintf(`# TailSQL — %s (%s mode)
spring.datasource.url=%s
spring.datasource.username=%s
spring.datasource.password=%s
spring.datasource.driver-class-name=org.h2.Driver
spring.datasource.hikari.maximum-pool-size=%d
spring.datasource.hikari.connection-timeout=%d
`, request.Database, request.Mode, propertyEscape(jdbcURL), propertyEscape(request.Username),
		propertyEscape(request.Password), MaxPoolSize, ConnectionTimeout)
}

func renderSpringYAML(request Request, jdbcURL, magicDNS string) string {
	magicDNSComment := ""
	if magicDNS != "" {
		magicDNSComment = fmt.Sprintf("\n# MagicDNS alternative: jdbc:h2:tcp://%s:%d/./%s", magicDNS, request.Port, request.Database)
	}
	return fmt.Sprintf(`# TailSQL — %s (%s mode)
spring:
  datasource:
    url: %s
    username: %s
    password: %s
    driver-class-name: org.h2.Driver
    hikari:
      maximum-pool-size: %d
      connection-timeout: %d%s
`, request.Database, request.Mode, yamlQuote(jdbcURL), yamlQuote(request.Username),
		yamlQuote(request.Password), MaxPoolSize, ConnectionTimeout, magicDNSComment)
}

func renderJDBCTest(request Request, jdbcURL, magicDNS string) string {
	magicDNSComment := ""
	if magicDNS != "" {
		magicDNSComment = fmt.Sprintf("\n * MagicDNS alternative: jdbc:h2:tcp://%s:%d/./%s", magicDNS, request.Port, request.Database)
	}
	return fmt.Sprintf(`import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.ResultSet;
import java.sql.Statement;

/**
 * Minimal TailSQL connectivity check for "%s" (%s mode).
 * JDBC URL: %s%s
 */
public class JdbcTest {

    public static void main(String[] args) throws Exception {
        String url = %s;
        String user = %s;
        String password = %s;

        try (Connection connection = DriverManager.getConnection(url, user, password);
             Statement statement = connection.createStatement();
             ResultSet rs = statement.executeQuery("SELECT 1")) {
            rs.next();
            System.out.println("Connected to H2 through TailSQL: SELECT 1 -> " + rs.getInt(1));
        }
    }
}
`, javaCommentSafe(request.Database), request.Mode, javaCommentSafe(jdbcURL), javaCommentSafe(magicDNSComment),
		javaQuote(jdbcURL), javaQuote(request.Username), javaQuote(request.Password))
}

func javaQuote(value string) string {
	var builder strings.Builder
	builder.WriteByte('"')
	for _, r := range value {
		switch r {
		case '\\':
			builder.WriteString(`\\`)
		case '"':
			builder.WriteString(`\"`)
		case '\n':
			builder.WriteString(`\n`)
		case '\r':
			builder.WriteString(`\r`)
		case '\t':
			builder.WriteString(`\t`)
		default:
			if r < 0x20 {
				builder.WriteString(fmt.Sprintf(`\u%04x`, r))
			} else {
				builder.WriteRune(r)
			}
		}
	}
	builder.WriteByte('"')
	return builder.String()
}

func javaCommentSafe(value string) string {
	replacer := strings.NewReplacer(
		"*/", "* /",
		"\r", " ",
		"\n", " ",
	)
	return replacer.Replace(value)
}

func xmlEscape(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(value)
}

func propertyEscape(value string) string {
	var builder strings.Builder
	for i, r := range value {
		switch r {
		case '\\':
			builder.WriteString(`\\`)
		case '\n':
			builder.WriteString(`\n`)
		case '\r':
			builder.WriteString(`\r`)
		case '\t':
			builder.WriteString(`\t`)
		case ' ':
			// The value follows a separator; only a leading space would be
			// discarded by java.util.Properties.
			if i == 0 {
				builder.WriteString(`\ `)
			} else {
				builder.WriteByte(' ')
			}
		default:
			if r < 0x20 {
				builder.WriteString(fmt.Sprintf(`\u%04x`, r))
			} else {
				builder.WriteRune(r)
			}
		}
	}
	return builder.String()
}

func yamlQuote(value string) string {
	var builder strings.Builder
	builder.WriteByte('"')
	for _, r := range value {
		switch r {
		case '\\':
			builder.WriteString(`\\`)
		case '"':
			builder.WriteString(`\"`)
		case '\n':
			builder.WriteString(`\n`)
		case '\r':
			builder.WriteString(`\r`)
		case '\t':
			builder.WriteString(`\t`)
		default:
			if r < 0x20 {
				builder.WriteString(fmt.Sprintf(`\u%04x`, r))
			} else {
				builder.WriteRune(r)
			}
		}
	}
	builder.WriteByte('"')
	return builder.String()
}

func poolName(database string) string {
	name := strings.NewReplacer("_", "-", " ", "-").Replace(database)
	if name == "" {
		return "tailsql"
	}
	return name
}

func sanitizeHostname(value string) string {
	trimmed := strings.TrimSuffix(strings.TrimSpace(value), ".")
	if trimmed == "" || !hostnamePattern.MatchString(trimmed) {
		return ""
	}
	return trimmed
}
