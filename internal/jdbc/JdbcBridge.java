import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.io.OutputStreamWriter;
import java.io.PrintWriter;
import java.io.StringReader;
import java.nio.charset.StandardCharsets;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.ResultSet;
import java.sql.ResultSetMetaData;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;
import java.util.Map;
import java.util.TreeMap;

/**
 * Tiny JDBC bridge executed by the host JVM against the bundled H2 jar.
 *
 * Credentials arrive through environment variables (never argv), SQL arrives on
 * stdin, and exactly one JSON object is written to stdout. This keeps SQL and
 * passwords out of the process command line while avoiding a Go H2 driver.
 *
 * Modes: ping | script | tables | exec
 * Exit codes: 0 success, 2 SQL error, 3 connection/configuration error.
 */
public final class JdbcBridge {
    private static final int MAX_ROWS = 500;
    private static final int MAX_VALUE_CHARS = 10_000;
    private static final int QUERY_TIMEOUT_SECONDS = 20;
    private static final PrintWriter OUT = new PrintWriter(
            new OutputStreamWriter(System.out, StandardCharsets.UTF_8), true);

    private JdbcBridge() {
    }

    public static void main(String[] args) {
        String mode = args.length > 0 ? args[0] : "exec";
        String url = env("TAILSQL_JDBC_URL");
        String user = env("TAILSQL_JDBC_USER");
        String password = env("TAILSQL_JDBC_PASSWORD");

        if (url.isEmpty()) {
            printError("TAILSQL_JDBC_URL is required", null, 0);
            System.exit(3);
        }

        long startedAt = System.nanoTime();
        int exitCode = 0;
        Connection connection = null;
        try {
            DriverManager.setLoginTimeout(15);
            connection = DriverManager.getConnection(url, user, password);
            ensureDurableWrites(connection);
            switch (mode) {
                case "ping":
                    ping(connection, startedAt);
                    break;
                case "script":
                    script(connection, readStdin(), startedAt);
                    break;
                case "tables":
                    tables(connection, startedAt);
                    break;
                case "exec":
                    exec(connection, readStdin(), startedAt);
                    break;
                default:
                    printError("Unknown bridge mode: " + mode, null, 0);
                    exitCode = 3;
            }
        } catch (SQLException e) {
            String state = e.getSQLState();
            printError(describe(e), state, e.getErrorCode());
            exitCode = state != null && (state.startsWith("08") || state.startsWith("28")) ? 3 : 2;
        } catch (Throwable t) {
            printError(t.getClass().getSimpleName() + ": " + safeMessage(t), null, 0);
            exitCode = 2;
        } finally {
            if (connection != null) {
                try {
                    connection.close();
                } catch (SQLException ignored) {
                    // A failed close must not hide the operation result.
                }
            }
        }
        OUT.flush();
        System.exit(exitCode);
    }

    /**
     * H2 applies the write delay while opening the database and resets it to
     * the connection property afterwards, so the URL carries it for fresh
     * opens and this statement covers databases another client already holds
     * open. Non-admin users cannot change it, which is safe to ignore.
     */
    private static void ensureDurableWrites(Connection connection) {
        try (Statement statement = connection.createStatement()) {
            statement.execute("SET WRITE_DELAY 0");
        } catch (SQLException ignored) {
            // The URL property remains the best-effort setting for this user.
        }
    }

    private static void ping(Connection connection, long startedAt) throws SQLException {
        try (Statement statement = connection.createStatement();
             ResultSet rs = statement.executeQuery("SELECT 1")) {
            rs.next();
        }
        String version = "";
        try {
            version = connection.getMetaData().getDatabaseProductVersion();
        } catch (SQLException ignored) {
            // Version display is best-effort.
        }
        StringBuilder json = new StringBuilder(128);
        json.append("{\"ok\":true,\"durationMs\":").append(elapsedMs(startedAt));
        json.append(",\"serverVersion\":").append(quote(version)).append('}');
        OUT.println(json);
    }

    private static void script(Connection connection, String sql, long startedAt) throws SQLException {
        if (sql.trim().isEmpty()) {
            throw new SQLException("SQL script is empty");
        }
        org.h2.tools.RunScript.execute(connection, new StringReader(sql));
        OUT.println("{\"ok\":true,\"durationMs\":" + elapsedMs(startedAt) + "}");
    }

    private static void exec(Connection connection, String sql, long startedAt) throws SQLException {
        if (sql.trim().isEmpty()) {
            throw new SQLException("SQL statement is empty");
        }

        boolean hasResultSet;
        long updateCount;
        List<Column> columns = new ArrayList<>();
        List<List<String>> rows = new ArrayList<>();
        boolean truncated = false;

        try (Statement statement = connection.createStatement()) {
            statement.setQueryTimeout(QUERY_TIMEOUT_SECONDS);
            statement.setMaxRows(MAX_ROWS + 1);
            hasResultSet = statement.execute(sql);
            if (hasResultSet) {
                try (ResultSet rs = statement.getResultSet()) {
                    ResultSetMetaData meta = rs.getMetaData();
                    int columnCount = meta.getColumnCount();
                    for (int i = 1; i <= columnCount; i++) {
                        columns.add(new Column(
                                meta.getColumnLabel(i),
                                meta.getColumnTypeName(i),
                                meta.isNullable(i) != ResultSetMetaData.columnNoNulls));
                    }
                    while (rs.next()) {
                        if (rows.size() >= MAX_ROWS) {
                            truncated = true;
                            break;
                        }
                        List<String> row = new ArrayList<>(columnCount);
                        for (int i = 1; i <= columnCount; i++) {
                            row.add(readValue(rs, i));
                        }
                        rows.add(row);
                    }
                }
                updateCount = -1;
            } else {
                updateCount = statement.getUpdateCount();
            }
        }

        StringBuilder json = new StringBuilder(4096);
        json.append("{\"ok\":true,\"durationMs\":").append(elapsedMs(startedAt));
        json.append(",\"hasResultSet\":").append(hasResultSet);
        json.append(",\"updateCount\":").append(updateCount);
        json.append(",\"rowCount\":").append(rows.size());
        json.append(",\"truncated\":").append(truncated);
        json.append(",\"columns\":[");
        for (int i = 0; i < columns.size(); i++) {
            if (i > 0) {
                json.append(',');
            }
            Column column = columns.get(i);
            json.append("{\"name\":").append(quote(column.name));
            json.append(",\"type\":").append(quote(column.type));
            json.append(",\"nullable\":").append(column.nullable).append('}');
        }
        json.append("],\"rows\":[");
        for (int r = 0; r < rows.size(); r++) {
            if (r > 0) {
                json.append(',');
            }
            json.append('[');
            List<String> row = rows.get(r);
            for (int c = 0; c < row.size(); c++) {
                if (c > 0) {
                    json.append(',');
                }
                json.append(quote(row.get(c)));
            }
            json.append(']');
        }
        json.append("]}");
        OUT.println(json);
    }

    private static void tables(Connection connection, long startedAt) throws SQLException {
        List<TableInfo> tables = new ArrayList<>();
        try (ResultSet rs = connection.getMetaData().getTables(null, null, "%", new String[]{"TABLE", "VIEW"})) {
            while (rs.next()) {
                String schema = rs.getString("TABLE_SCHEM");
                String name = rs.getString("TABLE_NAME");
                String type = rs.getString("TABLE_TYPE");
                if (schema == null || isSystemSchema(schema)) {
                    continue;
                }
                tables.add(new TableInfo(schema, name, type));
            }
        }
        for (TableInfo table : tables) {
            try (ResultSet rs = connection.getMetaData().getColumns(null, table.schema, table.name, "%")) {
                while (rs.next()) {
                    String name = rs.getString("COLUMN_NAME");
                    String type = rs.getString("TYPE_NAME");
                    int size = rs.getInt("COLUMN_SIZE");
                    boolean nullable = rs.getInt("NULLABLE") != ResultSetMetaData.columnNoNulls;
                    int ordinal = rs.getInt("ORDINAL_POSITION");
                    String defaultValue = rs.getString("COLUMN_DEF");
                    boolean autoIncrement = "YES".equalsIgnoreCase(rs.getString("IS_AUTOINCREMENT"));
                    boolean generated = "YES".equalsIgnoreCase(rs.getString("IS_GENERATEDCOLUMN"));
                    table.columns.add(new ColumnInfo(
                            name, type, size, nullable, ordinal,
                            defaultValue == null ? "" : defaultValue,
                            autoIncrement, generated));
                }
            }
            table.columns.sort(Comparator.comparingInt(c -> c.ordinal));

            Map<Integer, String> primaryKeys = new TreeMap<>();
            try (ResultSet rs = connection.getMetaData().getPrimaryKeys(null, table.schema, table.name)) {
                while (rs.next()) {
                    primaryKeys.put(rs.getInt("KEY_SEQ"), rs.getString("COLUMN_NAME"));
                }
            }
            table.primaryKey.addAll(primaryKeys.values());
        }
        tables.sort(Comparator.comparing((TableInfo t) -> t.schema).thenComparing(t -> t.name));

        StringBuilder json = new StringBuilder(4096);
        json.append("{\"ok\":true,\"durationMs\":").append(elapsedMs(startedAt));
        json.append(",\"tables\":[");
        for (int i = 0; i < tables.size(); i++) {
            if (i > 0) {
                json.append(',');
            }
            TableInfo table = tables.get(i);
            json.append("{\"schema\":").append(quote(table.schema));
            json.append(",\"name\":").append(quote(table.name));
            json.append(",\"type\":").append(quote(table.type));
            json.append(",\"columns\":[");
            for (int c = 0; c < table.columns.size(); c++) {
                if (c > 0) {
                    json.append(',');
                }
                ColumnInfo column = table.columns.get(c);
                json.append("{\"name\":").append(quote(column.name));
                json.append(",\"type\":").append(quote(column.type));
                json.append(",\"size\":").append(column.size);
                json.append(",\"nullable\":").append(column.nullable);
                json.append(",\"ordinal\":").append(column.ordinal);
                json.append(",\"default\":").append(quote(column.defaultValue));
                json.append(",\"autoIncrement\":").append(column.autoIncrement);
                json.append(",\"generated\":").append(column.generated).append('}');
            }
            json.append("],\"primaryKey\":[");
            for (int p = 0; p < table.primaryKey.size(); p++) {
                if (p > 0) {
                    json.append(',');
                }
                json.append(quote(table.primaryKey.get(p)));
            }
            json.append("]}");
        }
        json.append("]}");
        OUT.println(json);
    }

    private static String readValue(ResultSet rs, int index) {
        try {
            String value = rs.getString(index);
            if (value == null) {
                return null;
            }
            if (value.length() > MAX_VALUE_CHARS) {
                return value.substring(0, MAX_VALUE_CHARS) + "… [truncated]";
            }
            return value;
        } catch (SQLException e) {
            return "<unreadable: " + safeMessage(e) + ">";
        }
    }

    private static String readStdin() throws IOException {
        StringBuilder sb = new StringBuilder();
        try (BufferedReader reader = new BufferedReader(
                new InputStreamReader(System.in, StandardCharsets.UTF_8))) {
            char[] buffer = new char[8192];
            int read;
            while ((read = reader.read(buffer)) != -1) {
                sb.append(buffer, 0, read);
            }
        }
        return sb.toString();
    }

    private static boolean isSystemSchema(String schema) {
        String upper = schema.toUpperCase();
        return upper.equals("INFORMATION_SCHEMA") || upper.equals("PG_CATALOG");
    }

    private static long elapsedMs(long startedAt) {
        return Math.max(0, (System.nanoTime() - startedAt) / 1_000_000L);
    }

    private static String env(String name) {
        String value = System.getenv(name);
        return value == null ? "" : value;
    }

    private static String describe(SQLException e) {
        String message = safeMessage(e);
        return "SQLState " + (e.getSQLState() == null ? "n/a" : e.getSQLState())
                + " (code " + e.getErrorCode() + "): " + message;
    }

    private static String safeMessage(Throwable t) {
        String message = t.getMessage();
        return message == null ? t.getClass().getSimpleName() : message;
    }

    private static void printError(String message, String sqlState, int errorCode) {
        OUT.println("{\"ok\":false,\"error\":" + quote(message)
                + ",\"sqlState\":" + quote(sqlState)
                + ",\"errorCode\":" + errorCode + "}");
    }

    private static String quote(String value) {
        if (value == null) {
            return "null";
        }
        StringBuilder out = new StringBuilder(value.length() + 16);
        out.append('"');
        for (int i = 0; i < value.length(); i++) {
            char c = value.charAt(i);
            switch (c) {
                case '"':
                    out.append("\\\"");
                    break;
                case '\\':
                    out.append("\\\\");
                    break;
                case '\n':
                    out.append("\\n");
                    break;
                case '\r':
                    out.append("\\r");
                    break;
                case '\t':
                    out.append("\\t");
                    break;
                case '\b':
                    out.append("\\b");
                    break;
                case '\f':
                    out.append("\\f");
                    break;
                default:
                    if (c < 0x20) {
                        out.append(String.format("\\u%04x", (int) c));
                    } else {
                        out.append(c);
                    }
            }
        }
        out.append('"');
        return out.toString();
    }

    private static final class Column {
        final String name;
        final String type;
        final boolean nullable;

        Column(String name, String type, boolean nullable) {
            this.name = name;
            this.type = type;
            this.nullable = nullable;
        }
    }

    private static final class ColumnInfo {
        final String name;
        final String type;
        final int size;
        final boolean nullable;
        final int ordinal;
        final String defaultValue;
        final boolean autoIncrement;
        final boolean generated;

        ColumnInfo(String name, String type, int size, boolean nullable, int ordinal,
                   String defaultValue, boolean autoIncrement, boolean generated) {
            this.name = name;
            this.type = type;
            this.size = size;
            this.nullable = nullable;
            this.ordinal = ordinal;
            this.defaultValue = defaultValue;
            this.autoIncrement = autoIncrement;
            this.generated = generated;
        }
    }

    private static final class TableInfo {
        final String schema;
        final String name;
        final String type;
        final List<ColumnInfo> columns = new ArrayList<>();
        final List<String> primaryKey = new ArrayList<>();

        TableInfo(String schema, String name, String type) {
            this.schema = schema;
            this.name = name;
            this.type = type;
        }
    }
}
