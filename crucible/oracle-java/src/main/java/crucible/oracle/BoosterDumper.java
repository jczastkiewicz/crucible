package crucible.oracle;

import forge.CardStorageReader;
import forge.ImageKeys;
import forge.StaticData;
import forge.item.BoosterPack;
import forge.item.PaperCard;
import forge.item.SealedTemplate;
import forge.card.PrintSheet;
import forge.item.generation.BoosterGenerator;
import forge.util.Aggregates;
import forge.util.Lang;
import forge.util.Localizer;
import forge.util.MyRandom;

import java.io.PrintStream;
import java.lang.reflect.Field;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Random;

/**
 * Dumps what Booster Tutor's pack path (MakeCardEffect.java:81-89) does for
 * fixed seeds, for the Go port to be diffed against (ADR-0049's gate).
 *
 * <p>Three layers, one line per fact, tab separated:
 *
 * <pre>
 * T key                         the booster templates in StaticData.getBoosters() order
 * PICK seed key                 the template Aggregates.random picks under that seed
 * PACK seed key                 followed by one C line per card of the pack, in order
 * TPACK seed key                with "all": the pack of each template under seed = its index
 * C name|edition|collector|art|foil
 * SHEET key size                every print sheet the packs reached, then its S lines in
 * S name|edition|collector|art|foil weight    iteration order (toFlatList, runs merged)
 * </pre>
 *
 * <p>Usage: {@code BoosterDumper <forge-gui/res> <seed>[,<seed>...] [all]}. With {@code all} one
 * pack of every template is generated first, so every reachable sheet is dumped.
 */
public final class BoosterDumper {

    private BoosterDumper() {
    }

    public static void main(String[] args) throws Exception {
        if (args.length < 2) {
            System.err.println("usage: BoosterDumper <forge-gui/res> <seeds> [all]");
            System.exit(2);
        }
        String res = args[0];
        Lang.createInstance("en-US");
        // An empty image cache: some card lookups ask whether a picture exists.
        String none = java.nio.file.Files.createTempDirectory("no-images").toString();
        ImageKeys.initializeDirs(none, new java.util.HashMap<>(), none, none, none, none, none, none, none);
        Localizer.getInstance().initialize("en-US", res + "/languages/");
        CardStorageReader reader = new CardStorageReader(res + "/cardsfolder", null, false);
        CardStorageReader tokens = new CardStorageReader(res + "/tokenscripts", null, false);
        new StaticData(reader, tokens, null, null, res + "/editions", res + "/editions", res + "/blockdata",
                "", "LATEST_ART_ALL_EDITIONS", true, true, false, true, null);
        StaticData.instance().ensureAllCardsLoaded();
        PrintStream out = new PrintStream(System.out, false, StandardCharsets.UTF_8);

        for (SealedTemplate t : StaticData.instance().getBoosters()) {
            out.println("T\t" + t.getName());
        }
        if (args.length > 2 && args[2].equals("all")) {
            int i = 0;
            for (SealedTemplate t : StaticData.instance().getBoosters()) {
                MyRandom.setRandom(new Random(i++));
                try {
                    List<PaperCard> pack = new BoosterPack(t.getEdition(), t).getCards();
                    out.println("TPACK\t" + (i - 1) + "\t" + t.getName());
                    for (PaperCard c : pack) {
                        out.println("C\t" + card(c));
                    }
                } catch (RuntimeException e) {
                    out.println("ERR\t" + t.getName() + "\t" + e);
                }
            }
        }
        for (String seed : args[1].split(",")) {
            MyRandom.setRandom(new Random(Long.parseLong(seed)));
            SealedTemplate t = Aggregates.random(StaticData.instance().getBoosters());
            out.println("PICK\t" + seed + "\t" + t.getName());
            List<PaperCard> pack = new BoosterPack(t.getEdition(), t).getCards();
            out.println("PACK\t" + seed + "\t" + t.getName());
            for (PaperCard c : pack) {
                out.println("C\t" + card(c));
            }
        }
        dumpSheets(out);
        out.flush();
    }

    private static String card(PaperCard c) {
        return c.getName() + "|" + c.getEdition() + "|" + c.getCollectorNumber() + "|" + c.getArtIndex() + "|" + c.isFoil();
    }

    @SuppressWarnings("unchecked")
    private static void dumpSheets(PrintStream out) throws Exception {
        Field f = BoosterGenerator.class.getDeclaredField("cachedSheets");
        f.setAccessible(true);
        Map<String, PrintSheet> sheets = (Map<String, PrintSheet>) f.get(null);
        for (Map.Entry<String, PrintSheet> e : sheets.entrySet()) {
            PrintSheet ps = e.getValue();
            if (ps == null) {
                out.println("SHEET\t" + e.getKey() + "\tnull");
                continue;
            }
            Map<String, Integer> runs = new LinkedHashMap<>();
            List<String> order = new ArrayList<>();
            for (PaperCard c : ps.toFlatList()) {
                String k = card(c);
                if (!runs.containsKey(k)) {
                    order.add(k);
                }
                runs.merge(k, 1, Integer::sum);
            }
            out.println("SHEET\t" + e.getKey() + "\t" + order.size());
            for (String k : order) {
                out.println("S\t" + k + "\t" + runs.get(k));
            }
        }
    }
}
