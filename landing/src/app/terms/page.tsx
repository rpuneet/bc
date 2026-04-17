import { Nav } from "../_components/Nav";
import { Footer } from "../_components/Footer";

export default function TermsOfService() {
  return (
    <main className="min-h-screen bg-background">
      <Nav />

      <div className="mx-auto max-w-3xl px-6 pt-24 pb-16 lg:pt-32">
        <article>
          <header className="mb-12">
            <span className="inline-block rounded-full border border-primary/20 bg-primary/5 px-3 py-1 text-xs font-mono font-bold text-primary mb-4">
              LEGAL
            </span>
            <h1 className="text-4xl font-bold tracking-tight mb-2">
              Terms of Service
            </h1>
            <p className="text-sm text-muted-foreground font-mono">
              Last updated: February 2026
            </p>
          </header>

          <section className="space-y-8 text-[15px] leading-relaxed">
            <div>
              <h2 className="text-xl font-bold mb-3">1. Agreement to Terms</h2>
              <p className="text-muted-foreground">
                By accessing and using this website and the mycel services
                (&ldquo;Service&rdquo;), you accept and agree to be bound by and
                comply with these Terms and Conditions and our Privacy Policy.
                If you do not agree to abide by the above, please do not use
                this service.
              </p>
            </div>

            <div>
              <h2 className="text-xl font-bold mb-3">2. Use License</h2>
              <p className="text-muted-foreground mb-3">
                Permission is granted to temporarily download one copy of the
                materials (information or software) on mycel&apos;s website for
                personal, non-commercial transitory viewing only. This is the
                grant of a license, not a transfer of title, and under this
                license you may not:
              </p>
              <ul className="list-disc list-inside space-y-2 text-muted-foreground">
                <li>Modify or copy the materials</li>
                <li>
                  Use the materials for any commercial purpose or for any public
                  display
                </li>
                <li>
                  Attempt to decompile or reverse engineer any software
                  contained on the website
                </li>
                <li>
                  Remove any copyright or other proprietary notations from the
                  materials
                </li>
                <li>
                  Transfer the materials to another person or
                  &ldquo;mirror&rdquo; the materials on any other server
                </li>
                <li>
                  Violate any applicable laws or regulations in connection with
                  your access or use
                </li>
              </ul>
            </div>

            <div>
              <h2 className="text-xl font-bold mb-3">3. Disclaimer</h2>
              <p className="text-muted-foreground">
                The materials on mycel&apos;s website are provided on an &ldquo;as
                is&rdquo; basis. mycel makes no warranties, expressed or implied,
                and hereby disclaims and negates all other warranties including,
                without limitation, implied warranties or conditions of
                merchantability, fitness for a particular purpose, or
                non-infringement of intellectual property or other violation of
                rights.
              </p>
            </div>

            <div>
              <h2 className="text-xl font-bold mb-3">4. Limitations</h2>
              <p className="text-muted-foreground">
                In no event shall mycel or its suppliers be liable for any damages
                (including, without limitation, damages for loss of data or
                profit, or due to business interruption) arising out of the use
                or inability to use the materials on mycel&apos;s website, even if
                mycel or an authorized representative has been notified orally or
                in writing of the possibility of such damage.
              </p>
            </div>

            <div>
              <h2 className="text-xl font-bold mb-3">
                5. Accuracy of Materials
              </h2>
              <p className="text-muted-foreground">
                The materials appearing on mycel&apos;s website could include
                technical, typographical, or photographic errors. mycel does not
                warrant that any of the materials on its website are accurate,
                complete, or current. mycel may make changes to the materials
                contained on its website at any time without notice.
              </p>
            </div>

            <div>
              <h2 className="text-xl font-bold mb-3">6. Links</h2>
              <p className="text-muted-foreground">
                mycel has not reviewed all of the sites linked to its website and
                is not responsible for the contents of any such linked site. The
                inclusion of any link does not imply endorsement by mycel of the
                site. Use of any such linked website is at the user&apos;s own
                risk.
              </p>
            </div>

            <div>
              <h2 className="text-xl font-bold mb-3">7. Modifications</h2>
              <p className="text-muted-foreground">
                mycel may revise these terms of service for its website at any time
                without notice. By using this website, you are agreeing to be
                bound by the then current version of these terms of service.
              </p>
            </div>

            <div>
              <h2 className="text-xl font-bold mb-3">8. Governing Law</h2>
              <p className="text-muted-foreground">
                These terms and conditions are governed by and construed in
                accordance with the laws of the United States, and you
                irrevocably submit to the exclusive jurisdiction of the courts
                in that location.
              </p>
            </div>

            <div>
              <h2 className="text-xl font-bold mb-3">9. Acceptable Use</h2>
              <p className="text-muted-foreground mb-3">
                You agree not to use the Service:
              </p>
              <ul className="list-disc list-inside space-y-2 text-muted-foreground">
                <li>
                  In any way that violates any applicable law or regulation
                </li>
                <li>To transmit any harmful or malicious code</li>
                <li>
                  To impersonate or attempt to impersonate any person or entity
                </li>
                <li>
                  To engage in any conduct that restricts or inhibits
                  anyone&apos;s use or enjoyment of the Service
                </li>
                <li>To harass, abuse, or harm others</li>
              </ul>
            </div>

            <div>
              <h2 className="text-xl font-bold mb-3">
                10. Contact Information
              </h2>
              <p className="text-muted-foreground">
                If you have any questions about these Terms of Service, please
                contact us at:
              </p>
              <p className="text-muted-foreground mt-3">
                <strong className="text-foreground">mycel</strong>
                <br />
                Email:{" "}
                <a
                  href="mailto:puneet@mycel.dev"
                  className="text-primary hover:underline"
                >
                  puneet@mycel.dev
                </a>
              </p>
            </div>
          </section>
        </article>
      </div>

      <Footer />
    </main>
  );
}
