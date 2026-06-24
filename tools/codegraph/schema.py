"""Kuzu schema definitions for the Streamclone code graph."""

from __future__ import annotations

import kuzu


def create_schema(conn: kuzu.Connection) -> None:
    statements = [
        """
        CREATE NODE TABLE File(
            path STRING,
            abspath STRING,
            language STRING,
            sha256 STRING,
            byte_count INT64,
            indexed_at INT64,
            PRIMARY KEY(path)
        )
        """,
        """
        CREATE NODE TABLE Function(
            id STRING,
            name STRING,
            qualified_name STRING,
            file_path STRING,
            start_line INT64,
            end_line INT64,
            start_byte INT64,
            end_byte INT64,
            receiver STRING,
            PRIMARY KEY(id)
        )
        """,
        """
        CREATE NODE TABLE Class(
            id STRING,
            name STRING,
            qualified_name STRING,
            file_path STRING,
            start_line INT64,
            end_line INT64,
            start_byte INT64,
            end_byte INT64,
            receiver STRING,
            PRIMARY KEY(id)
        )
        """,
        """
        CREATE NODE TABLE Interface(
            id STRING,
            name STRING,
            qualified_name STRING,
            file_path STRING,
            start_line INT64,
            end_line INT64,
            start_byte INT64,
            end_byte INT64,
            receiver STRING,
            PRIMARY KEY(id)
        )
        """,
        """
        CREATE NODE TABLE ImportModule(
            id STRING,
            name STRING,
            path STRING,
            local_path STRING,
            PRIMARY KEY(id)
        )
        """,
        """
        CREATE NODE TABLE Route(
            id STRING,
            method STRING,
            path STRING,
            handler STRING,
            file_path STRING,
            line INT64,
            source STRING,
            PRIMARY KEY(id)
        )
        """,
        """
        CREATE NODE TABLE Test(
            id STRING,
            name STRING,
            file_path STRING,
            target_file STRING,
            target_symbol STRING,
            line INT64,
            PRIMARY KEY(id)
        )
        """,
        """
        CREATE NODE TABLE Service(
            id STRING,
            name STRING,
            description STRING,
            keywords STRING,
            PRIMARY KEY(id)
        )
        """,
        "CREATE REL TABLE DEFINES(FROM File TO Function, FROM File TO Class, FROM File TO Interface)",
        "CREATE REL TABLE IMPORTS(FROM File TO ImportModule, alias STRING, local_path STRING, line INT64)",
        "CREATE REL TABLE CALLS(FROM Function TO Function, FROM Function TO Class, call_expr STRING, line INT64, resolved_by STRING)",
        "CREATE REL TABLE INHERITS_FROM(FROM Class TO Class, FROM Class TO Interface, FROM Interface TO Interface, reason STRING, line INT64)",
        "CREATE REL TABLE HANDLES(FROM Route TO Function, handler_expr STRING)",
        "CREATE REL TABLE TESTS(FROM Test TO File, FROM Test TO Function, reason STRING)",
        "CREATE REL TABLE BELONGS_TO(FROM File TO Service, FROM Function TO Service, FROM Route TO Service)",
        "CREATE REL TABLE CLIENT_CALLS(FROM File TO Route, call_expr STRING, line INT64)",
    ]
    for statement in statements:
        conn.execute(statement)
