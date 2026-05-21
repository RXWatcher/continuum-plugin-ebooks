import { useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Trash2, Upload } from "lucide-react";
import {
  deleteCustomFont,
  uploadCustomFont,
  type CustomFont,
} from "@/lib/api";
import { Button } from "@/components/ui/button";

// CustomFontUploader is the inline reader-side surface for uploading
// .ttf/.otf/.woff/.woff2 fonts + deleting them. Lives inside the
// reader's settings drawer so the user can change reading fonts
// without leaving the page.
//
// The font-family the SPA uses == the user-entered name. The
// server stores the name verbatim; @font-face emitted in Reader.tsx
// pairs it with the served bytes.
export default function CustomFontUploader({ fonts }: { fonts: CustomFont[] }) {
  const qc = useQueryClient();
  const fileRef = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);

  const upload = useMutation({
    mutationFn: (file: File) =>
      uploadCustomFont(file, file.name.replace(/\.[^.]+$/, "")),
    onSuccess: () => {
      toast.success("Font uploaded");
      qc.invalidateQueries({ queryKey: ["custom-fonts"] });
    },
    onError: (err) => toast.error(`Upload failed: ${err}`),
    onSettled: () => setBusy(false),
  });

  const remove = useMutation({
    mutationFn: (id: string) => deleteCustomFont(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["custom-fonts"] }),
    onError: (err) => toast.error(`Delete failed: ${err}`),
  });

  return (
    <div className="space-y-2">
      <input
        ref={fileRef}
        type="file"
        accept=".ttf,.otf,.woff,.woff2,font/ttf,font/otf,font/woff,font/woff2"
        className="hidden"
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (!f) return;
          setBusy(true);
          upload.mutate(f);
          e.target.value = "";
        }}
      />
      <Button
        type="button"
        size="sm"
        variant="secondary"
        onClick={() => fileRef.current?.click()}
        disabled={busy}
      >
        <Upload className="size-4" />
        <span className="ml-1">Upload font</span>
      </Button>
      {fonts.length > 0 && (
        <ul className="space-y-1">
          {fonts.map((f) => (
            <li key={f.id} className="flex items-center justify-between text-xs">
              <span className="text-muted-foreground truncate">{f.name}</span>
              <button
                type="button"
                onClick={() => remove.mutate(f.id)}
                className="text-muted-foreground hover:text-destructive size-6 p-0"
                title="Remove font"
              >
                <Trash2 className="size-3" />
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
