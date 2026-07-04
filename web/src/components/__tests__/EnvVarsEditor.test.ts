import { describe, it, expect } from "vitest";
import { insertSecretRef, isValidEnvKey, secretFilterTerm } from "../EnvVarsEditor";

describe("isValidEnvKey", () => {
  it("accepts valid env var names", () => {
    for (const k of ["FOO", "_FOO", "foo_bar", "A1", "HTTP_PROXY"]) {
      expect(isValidEnvKey(k)).toBe(true);
    }
  });

  it("rejects malformed names", () => {
    for (const k of ["", "1FOO", "FOO-BAR", "FOO BAR", "FOO=1", "${X}"]) {
      expect(isValidEnvKey(k)).toBe(false);
    }
  });
});

describe("insertSecretRef", () => {
  it("appends a reference to a plain value", () => {
    expect(insertSecretRef("", "TOKEN")).toBe("${secret:TOKEN}");
    expect(insertSecretRef("abc", "TOKEN")).toBe("abc${secret:TOKEN}");
  });

  it("replaces a trailing partial ${ being typed", () => {
    expect(insertSecretRef("${", "TOKEN")).toBe("${secret:TOKEN}");
    expect(insertSecretRef("pre-${sec", "TOKEN")).toBe("pre-${secret:TOKEN}");
    expect(insertSecretRef("pre-${secret:TOK", "TOKEN")).toBe("pre-${secret:TOKEN}");
  });

  it("leaves completed references alone and appends", () => {
    expect(insertSecretRef("${secret:A}", "B")).toBe("${secret:A}${secret:B}");
  });
});

describe("secretFilterTerm", () => {
  it("strips a typed secret: prefix", () => {
    expect(secretFilterTerm("secret:GIT")).toBe("GIT");
  });

  it("treats a partial prefix as no filter", () => {
    expect(secretFilterTerm("sec")).toBe("");
    expect(secretFilterTerm("")).toBe("");
  });

  it("passes through non-prefix terms", () => {
    expect(secretFilterTerm("GIT")).toBe("GIT");
  });
});
