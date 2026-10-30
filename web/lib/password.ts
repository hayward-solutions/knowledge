export const MIN_PASSWORD_LENGTH = 12;

export const PASSWORD_HINT =
  "At least 12 characters with upper case, lower case, a digit, and a symbol.";

export function passwordPolicyOk(pw: string): boolean {
  return (
    pw.length >= MIN_PASSWORD_LENGTH &&
    /[A-Z]/.test(pw) &&
    /[a-z]/.test(pw) &&
    /[0-9]/.test(pw) &&
    /[^A-Za-z0-9]/.test(pw)
  );
}
