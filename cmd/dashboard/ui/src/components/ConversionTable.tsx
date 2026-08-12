interface ConversionRow {
  total_applied: number;
  interviews: number;
  rejections: number;
  pending: number;
  interview_rate_pct: string;
}

interface ConversionTableProps<Row extends ConversionRow> {
  caption: string;
  keyHeader: string;
  rows: Row[];
  rowKey: (row: Row) => string;
}

export function ConversionTable<Row extends ConversionRow>({
  caption,
  keyHeader,
  rows,
  rowKey,
}: ConversionTableProps<Row>) {
  if (rows.length === 0) return null;

  return (
    <div className="conversion-breakdown">
      <table>
        <caption>{caption}</caption>
        <thead>
          <tr>
            <th scope="col">{keyHeader}</th>
            <th scope="col">Applied</th>
            <th scope="col">Interviews</th>
            <th scope="col">Rejections</th>
            <th scope="col">Pending</th>
            <th scope="col">Rate</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={rowKey(row)}>
              <th scope="row">{rowKey(row)}</th>
              <td>{row.total_applied}</td>
              <td>{row.interviews}</td>
              <td>{row.rejections}</td>
              <td>{row.pending}</td>
              <td>{row.interview_rate_pct || '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
