/**
 * ChannelCompose — send text and attach files on a gateway channel.
 * MessageContent renders `[file:ID]` tokens; this control is how they
 * get into the draft (paperclip / drag-drop → POST /api/files/upload).
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { ChannelCompose, fileRef } from "../ChannelCompose";

const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;

function jsonResponse(body: unknown, status = 200) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 201 ? "Created" : "OK",
    json: () => Promise.resolve(body),
  } as Response);
}

beforeEach(() => {
  fetchMock.mockReset();
});

describe("ChannelCompose", () => {
  it("POSTs typed text to /api/apps/channels/send and clears the draft", async () => {
    fetchMock.mockReturnValue(jsonResponse({ sent: true }));
    const onSent = vi.fn();
    render(<ChannelCompose channelName="slack:general" onSent={onSent} />);

    fireEvent.change(screen.getByLabelText("Message"), { target: { value: "hello channel" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/apps/channels/send",
        expect.objectContaining({ method: "POST" }),
      );
    });
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(JSON.parse(init.body as string)).toEqual({
      channel: "slack:general",
      message: "hello channel",
      sender: "web",
    });
    await waitFor(() => {
      expect(screen.getByLabelText("Message")).toHaveValue("");
    });
    expect(onSent).toHaveBeenCalledTimes(1);
  });

  it("sends on Enter and keeps the draft on Shift+Enter", async () => {
    fetchMock.mockReturnValue(jsonResponse({ sent: true }));
    render(<ChannelCompose channelName="telegram:alerts" />);

    const box = screen.getByLabelText("Message");
    fireEvent.change(box, { target: { value: "line one" } });
    fireEvent.keyDown(box, { key: "Enter", shiftKey: true });
    expect(fetchMock).not.toHaveBeenCalled();
    expect(box).toHaveValue("line one");

    fireEvent.keyDown(box, { key: "Enter", shiftKey: false });
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  });

  it("keeps the draft and surfaces a warning when the channel has no route", async () => {
    fetchMock.mockReturnValue(jsonResponse({ sent: false }));
    render(<ChannelCompose channelName="engineering" />);

    fireEvent.change(screen.getByLabelText("Message"), { target: { value: "ping" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByRole("status")).toHaveTextContent("No outbound route");
    expect(screen.getByLabelText("Message")).toHaveValue("ping");
  });

  it("uploads a paperclip file and inserts [file:ID] into the draft", async () => {
    fetchMock.mockReturnValue(
      jsonResponse(
        { id: "abc123", filename: "note.txt", mime_type: "text/plain", size: 4, channel: "slack:general", sender: "web", created_at: "2026-01-01T00:00:00Z" },
        201,
      ),
    );
    render(<ChannelCompose channelName="slack:general" />);

    const file = new File(["hi\n"], "note.txt", { type: "text/plain" });
    fireEvent.change(screen.getByTestId("channel-file-input"), {
      target: { files: [file] },
    });

    await waitFor(() => {
      expect(screen.getByLabelText("Message")).toHaveValue(fileRef("abc123"));
    });
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/files/upload");
    expect(init.method).toBe("POST");
    expect(init.body).toBeInstanceOf(FormData);
    const body = init.body as FormData;
    expect(body.get("channel")).toBe("slack:general");
    expect(body.get("sender")).toBe("web");
    expect(body.get("file")).toBeInstanceOf(File);
  });

  it("uploads a dropped file the same way as the paperclip", async () => {
    fetchMock.mockReturnValue(
      jsonResponse(
        { id: "drop1", filename: "shot.png", mime_type: "image/png", size: 8, channel: "slack:general", sender: "web", created_at: "2026-01-01T00:00:00Z" },
        201,
      ),
    );
    render(<ChannelCompose channelName="slack:general" />);

    const file = new File(["pngdata"], "shot.png", { type: "image/png" });
    fireEvent.drop(screen.getByTestId("channel-compose"), {
      dataTransfer: { files: [file], types: ["Files"] },
    });

    await waitFor(() => {
      expect(screen.getByLabelText("Message")).toHaveValue(fileRef("drop1"));
    });
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/files/upload");
  });
});
