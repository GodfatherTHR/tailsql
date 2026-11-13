import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.io.OutputStreamWriter;
import java.io.PrintWriter;
import java.io.StringReader;
import java.nio.charset.StandardCharsets;
import java.sql.Connection;
import java.sql.DatabaseMetaData;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.ResultSetMetaData;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.ArrayList;
import java.util.Base64;
import java.util.Comparator;
import java.util.LinkedHashMap;
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
 * Modes: ping | script | tables | exec | query
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
        String url = env("HYSQL_JDBC_URL");
        String user = env("HYSQL_JDBC_USER");
        String password = env("HYSQL_JDBC_PASSWORD");

        if (url.isEmpty()) {
            printError("HYSQL_JDBC_URL is required", null, 0);
            System.exit(3);
        }

        BufferedReader reader = new BufferedReader(
                new InputStreamReader(System.in, StandardCharsets.UTF_8));
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
                    script(connection, readAll(reader), startedAt);
                    break;
                case "tables":
                    tables(connection, startedAt);
                    break;
                case "exec":
                    exec(connection, readAll(reader), startedAt);
                    break;
                case "query":
                    query(connection, reader, startedAt);
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
        try (Statement statement = connection.createStatement()) {
            statement.setQueryTimeout(QUERY_TIMEOUT_SECONDS);
            statement.setMaxRows(MAX_ROWS + 1);
            boolean hasResultSet = statement.execute(sql);
            emitStatementResult(statement, hasResultSet, startedAt);
        }
    }

    /**
     * Prepared-statement mode. Input is a small framed protocol so values never
     * touch argv or interpolated SQL:
     *
     * <pre>
     * n=&lt;count&gt;
     * S&lt;base64 UTF-8 value&gt;   (or N for NULL, once per parameter)
     * SQL
     * &lt;sql, may span lines&gt;
     * </pre>
     */
    private static void query(Connection connection, BufferedReader reader, long startedAt)
            throws SQLException, IOException {
        String header = reader.readLine();
        if (header == null || !header.startsWith("n=")) {
            throw new SQLException("Invalid query parameter header");
        }
        int count;
        try {
            count = Integer.parseInt(header.substring(2).trim());
        } catch (NumberFormatException e) {
            throw new SQLException("Invalid query parameter count");
        }
        if (count < 0 || count > 10_000) {
            throw new SQLException("Invalid query parameter count");
        }
        List<String> params = new ArrayList<>(count);
        for (int index = 0; index < count; index++) {
            String line = reader.readLine();
            if (line == null) {
                throw new SQLException("Unexpected end of query parameter input");
            }
            if ("N".equals(line)) {
                params.add(null);
            } else if (line.startsWith("S")) {
                try {
                    params.add(new String(Base64.getDecoder().decode(line.substring(1)), StandardCharsets.UTF_8));
                } catch (IllegalArgumentException e) {
                    throw new SQLException("Invalid query parameter encoding");
                }
            } else {
                throw new SQLException("Invalid query parameter encoding");
            }
        }
        String marker = reader.readLine();
        if (!"SQL".equals(marker)) {
            throw new SQLException("Missing SQL marker in query input");
        }
        String sql = readAll(reader).trim();
        if (sql.isEmpty()) {
            throw new SQLException("SQL statement is empty");
        }

        try (PreparedStatement statement = connection.prepareStatement(sql)) {
            statement.setQueryTimeout(QUERY_TIMEOUT_SECONDS);
            statement.setMaxRows(MAX_ROWS + 1);
            for (int index = 0; index < params.size(); index++) {
                statement.setString(index + 1, params.get(index));
            }
            boolean hasResultSet = statement.execute();
            emitStatementResult(statement, hasResultSet, startedAt);
        }
    }

    private static void emitStatementResult(Statement statement, boolean hasResultSet, long startedAt)
            throws SQLException {
        long updateCount;
        List<Column> columns = new ArrayList<>();
        List<List<String>> rows = new ArrayList<>();
        boolean truncated = false;

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
        DatabaseMetaData meta = connection.getMetaData();
        List<TableInfo> tables = new ArrayList<>();
        try (ResultSet rs = meta.getTables(null, null, "%", new String[]{"TABLE", "VIEW"})) {
            while (rs.next()) {
                String schema = rs.getString("TABLE_SCHEM");
                String name = rs.getString("TABLE_NAME");
                String type = rs.getString("TABLE_TYPE");
                if (schema == null || isSystemSchema(schema)) {
                    continue;
                }
                tables.add(new TableInfo(schema, name, type, safe(rs.getString("REMARKS"))));
            }
        }
        for (TableInfo table : tables) {
            try (ResultSet rs = meta.getColumns(null, table.schema, table.name, "%")) {
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
                            autoIncrement, generated, safe(rs.getString("REMARKS"))));
                }
            }
            table.columns.sort(Comparator.comparingInt(c -> c.ordinal));

            Map<Integer, String> primaryKeys = new TreeMap<>();
            try (ResultSet rs = meta.getPrimaryKeys(null, table.schema, table.name)) {
                while (rs.next()) {
                    primaryKeys.put(rs.getInt("KEY_SEQ"), rs.getString("COLUMN_NAME"));
                }
            }
            table.primaryKey.addAll(primaryKeys.values());

            try (ResultSet rs = meta.getImportedKeys(null, table.schema, table.name)) {
                while (rs.next()) {
                    table.foreignKeys.add(new ForeignKeyInfo(
                            safe(rs.getString("FK_NAME")),
                            safe(rs.getString("FKCOLUMN_NAME")),
                            safe(rs.getString("PKTABLE_SCHEM")),
                            safe(rs.getString("PKTABLE_NAME")),
                            safe(rs.getString("PKCOLUMN_NAME")),
                            ruleText(rs.getInt("UPDATE_RULE")),
                            ruleText(rs.getInt("DELETE_RULE"))));
                }
            }
            table.foreignKeys.sort(Comparator
                    .comparing((ForeignKeyInfo fk) -> fk.name)
                    .thenComparing(fk -> fk.column));

            Map<String, IndexInfo> indexes = new LinkedHashMap<>();
            try (ResultSet rs = meta.getIndexInfo(null, table.schema, table.name, false, false)) {
                while (rs.next()) {
                    if (rs.getShort("TYPE") == DatabaseMetaData.tableIndexStatistic) {
                        continue;
                    }
                    String indexName = rs.getString("INDEX_NAME");
                    if (indexName == null) {
                        continue;
                    }
                    IndexInfo index = indexes.get(indexName);
                    if (index == null) {
                        index = new IndexInfo(indexName, !rs.getBoolean("NON_UNIQUE"));
                        indexes.put(indexName, index);
                    }
                    String column = rs.getString("COLUMN_NAME");
                    if (column != null) {
                        index.columns.add(column);
                    }
                }
            }
            table.indexes.addAll(indexes.values());
            table.indexes.sort(Comparator.comparing(index -> index.name));
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
            json.append(",\"remarks\":").append(quote(table.remarks));
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
                json.append(",\"generated\":").append(column.generated);
                json.append(",\"remarks\":").append(quote(column.remarks)).append('}');
            }
            json.append("],\"primaryKey\":[");
            for (int p = 0; p < table.primaryKey.size(); p++) {
                if (p > 0) {
                    json.append(',');
                }
                json.append(quote(table.primaryKey.get(p)));
            }
            json.append("],\"foreignKeys\":[");
            for (int f = 0; f < table.foreignKeys.size(); f++) {
                if (f > 0) {
                    json.append(',');
                }
                ForeignKeyInfo fk = table.foreignKeys.get(f);
                json.append("{\"name\":").append(quote(fk.name));
                json.append(",\"column\":").append(quote(fk.column));
                json.append(",\"referencedSchema\":").append(quote(fk.referencedSchema));
                json.append(",\"referencedTable\":").append(quote(fk.referencedTable));
                json.append(",\"referencedColumn\":").append(quote(fk.referencedColumn));
                json.append(",\"onUpdate\":").append(quote(fk.onUpdate));
                json.append(",\"onDelete\":").append(quote(fk.onDelete)).append('}');
            }
            json.append("],\"indexes\":[");
            for (int x = 0; x < table.indexes.size(); x++) {
                if (x > 0) {
                    json.append(',');
                }
                IndexInfo index = table.indexes.get(x);
                json.append("{\"name\":").append(quote(index.name));
                json.append(",\"unique\":").append(index.unique);
                json.append(",\"columns\":[");
                for (int c = 0; c < index.columns.size(); c++) {
                    if (c > 0) {
                        json.append(',');
                    }
                    json.append(quote(index.columns.get(c)));
                }
                json.append("]}");
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

    private static String readAll(BufferedReader reader) throws IOException {
        StringBuilder sb = new StringBuilder();
        char[] buffer = new char[8192];
        int read;
        while ((read = reader.read(buffer)) != -1) {
            sb.append(buffer, 0, read);
        }
        return sb.toString();
    }

    private static String ruleText(int rule) {
        switch (rule) {
            case DatabaseMetaData.importedKeyCascade:
                return "CASCADE";
            case DatabaseMetaData.importedKeyRestrict:
                return "RESTRICT";
            case DatabaseMetaData.importedKeySetNull:
                return "SET NULL";
            case DatabaseMetaData.importedKeyNoAction:
                return "NO ACTION";
            case DatabaseMetaData.importedKeySetDefault:
                return "SET DEFAULT";
            default:
                return "";
        }
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

    private static String safe(String value) {
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
        final String remarks;

        ColumnInfo(String name, String type, int size, boolean nullable, int ordinal,
                   String defaultValue, boolean autoIncrement, boolean generated, String remarks) {
            this.name = name;
            this.type = type;
            this.size = size;
            this.nullable = nullable;
            this.ordinal = ordinal;
            this.defaultValue = defaultValue;
            this.autoIncrement = autoIncrement;
            this.generated = generated;
            this.remarks = remarks;
        }
    }

    private static final class ForeignKeyInfo {
        final String name;
        final String column;
        final String referencedSchema;
        final String referencedTable;
        final String referencedColumn;
        final String onUpdate;
        final String onDelete;

        ForeignKeyInfo(String name, String column, String referencedSchema, String referencedTable,
                       String referencedColumn, String onUpdate, String onDelete) {
            this.name = name;
            this.column = column;
            this.referencedSchema = referencedSchema;
            this.referencedTable = referencedTable;
            this.referencedColumn = referencedColumn;
            this.onUpdate = onUpdate;
            this.onDelete = onDelete;
        }
    }

    private static final class IndexInfo {
        final String name;
        final boolean unique;
        final List<String> columns = new ArrayList<>();

        IndexInfo(String name, boolean unique) {
            this.name = name;
            this.unique = unique;
        }
    }

    private static final class TableInfo {
        final String schema;
        final String name;
        final String type;
        final String remarks;
        final List<ColumnInfo> columns = new ArrayList<>();
        final List<String> primaryKey = new ArrayList<>();
        final List<ForeignKeyInfo> foreignKeys = new ArrayList<>();
        final List<IndexInfo> indexes = new ArrayList<>();

        TableInfo(String schema, String name, String type, String remarks) {
            this.schema = schema;
            this.name = name;
            this.type = type;
            this.remarks = remarks;
        }
    }
}
