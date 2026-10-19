// Classifies the result of one real read-only dependency client. The caller
// sends the wrong synthetic password over stdin and never records raw output.
export function authenticationRejected(result, kind) {
  if (kind !== 'postgres' && kind !== 'redis') throw new Error('unsupported credential probe');
  if (!result || !Number.isInteger(result.code) || result.code <= 0) return false;
  const text = `${typeof result.stdout === 'string' ? result.stdout : ''}\n${typeof result.stderr === 'string' ? result.stderr : ''}`;
  if (/timeout|timed out|connection refused|no route|certificate|tls handshake|ssl_connect|could not translate|name or service not known|forbidden/i.test(text)) return false;
  return kind === 'postgres'
    ? /password authentication failed/i.test(text)
    : /WRONGPASS|NOAUTH|invalid username-password pair/i.test(text);
}
