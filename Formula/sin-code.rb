class SinCode < Formula
  desc "SIN-Code unified toolchain — 13 analysis and manipulation tools in one binary"
  homepage "https://github.com/OpenSIN-Code/SIN-Code-Bundle"
  version "1.0.4"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.4-sin-code/sin-code-darwin-arm64.tar.gz"
      sha256 "81ffe334308550d21d6fbeb6912d3228fbef79818acfba781e31878091f5e111"
    else
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.4-sin-code/sin-code-darwin-amd64.tar.gz"
      sha256 "c55266d9cb308be06cb2b7087d8ff9c3320fb47c58977b5f315174647a5bbbcc"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.4-sin-code/sin-code-linux-arm64.tar.gz"
      sha256 "c4d91e628434dbd7eef408ebe1683dfae03408d51b1fa57bb50a4aade5aa744c"
    else
      url "https://github.com/OpenSIN-Code/SIN-Code-Bundle/releases/download/v1.0.4-sin-code/sin-code-linux-amd64.tar.gz"
      sha256 "108b2f421b7471b64bc5dd6cabc64caaa634a1771f610dd4112c41efc6eb0bba"
    end
  end

  def install
    bin.install "sin-code"
  end

  test do
    system "#{bin}/sin-code", "--version"
  end
end
