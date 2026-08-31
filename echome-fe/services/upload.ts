export async function uploadFile(file: File): Promise<string> {
  const formData = new FormData();
  formData.append("file", file);

  const res = await fetch("/v1/api/files", {
    method: "POST",
    body: formData,
  });
  const json = await res.json().catch(() => ({}));
  const data = json.data ?? json;
  if (!res.ok || !data.url) {
    const message =
      (typeof data.message === "string" && data.message) ||
      (typeof json.error?.message === "string" && json.error.message) ||
      "Upload failed";
    throw new Error(message);
  }
  return data.url as string;
}

export default uploadFile;
